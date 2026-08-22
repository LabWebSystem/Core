package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type featureRunner struct {
	gitError bool
	mu       sync.Mutex
	args     [][]string
}

func (r *featureRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.args = append(r.args, append([]string{name}, args...))
	r.mu.Unlock()

	if name == "git" {
		if r.gitError {
			return nil, fmt.Errorf("git fixture failure")
		}
		return (OSRunner{}).Run(ctx, name, args...)
	}
	if name != "docker" {
		return nil, fmt.Errorf("未対応のコマンドです: %s", name)
	}

	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, " ps -a "):
		return nil, nil
	case strings.Contains(joined, "network inspect"):
		return nil, fmt.Errorf("No such network")
	case strings.Contains(joined, "compose") && strings.Contains(joined, "config"):
		return []byte(`{"services":{"web":{"image":"nginx"}}}`), nil
	default:
		return nil, nil
	}
}

func (r *featureRunner) hasDockerComposeUp() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, args := range r.args {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "compose") && strings.Contains(joined, " up -d") {
			return true
		}
	}
	return false
}

func featureRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("テストファイルの位置を取得できません")
	}
	return filepath.Dir(filepath.Dir(file))
}

func prepareFeatureRepository(t *testing.T, fixture, name string) string {
	t.Helper()
	tmp := t.TempDir()
	repository := filepath.Join(tmp, name+".git")
	config := filepath.Join(tmp, "gitconfig")
	url := "https://github.com/test/" + name
	command := exec.Command(filepath.Join(featureRepositoryRoot(t), "scripts", "test-app-fixture.sh"), "prepare", fixture, repository, config, url)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture repositoryを作成できません: %v\n%s", err, output)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	return url
}

func newFeatureServer(t *testing.T, runner *featureRunner) (*sql.DB, *RuntimeExecutor, *httptest.Server) {
	t.Helper()
	tmp := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(tmp, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &RuntimeExecutor{DB: db, Root: filepath.Join(tmp, "apps"), Runner: runner}
	e.Derived = &DerivedManager{
		DB:            db,
		GeneratedDir:  filepath.Join(tmp, "generated"),
		BaseDomain:    "example.internal",
		PublicAddress: "192.0.2.10",
	}
	server := httptest.NewServer(NewServer(db, e.Run).Handler())
	t.Cleanup(server.Close)
	return db, e, server
}

func waitFeatureOperation(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var state, message string
		if err := db.QueryRow(`SELECT state,error_message FROM operations WHERE id=?`, strings.TrimPrefix(name, "operations/")).Scan(&state, &message); err == nil {
			if state == "SUCCEEDED" || state == "FAILED" {
				return state + "\x00" + message
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Operationが完了しませんでした")
	return ""
}

func TestFTV1_020RegisterValidApp(t *testing.T) {
	url := prepareFeatureRepository(t, "valid", "lws-valid")
	runner := &featureRunner{}
	db, executor, server := newFeatureServer(t, runner)

	body := strings.NewReader(fmt.Sprintf(`{"repositoryUrl":%q,"ref":"main","subdomain":"valid","requestId":"550e8400-e29b-41d4-a716-446655440201"}`, url))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/applications", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("登録受付に失敗しました: status=%d body=%s", response.Code, response.Body.String())
	}
	var accepted map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	result := waitFeatureOperation(t, db, accepted["name"])
	if !strings.HasPrefix(result, "SUCCEEDED\x00") {
		t.Fatalf("登録Operationが成功しませんでした: %q", result)
	}

	var state, observed, name, service string
	if err := db.QueryRow(`SELECT registration_state,observed_state,manifest_name,manifest_service FROM applications WHERE subdomain='valid'`).Scan(&state, &observed, &name, &service); err != nil {
		t.Fatal(err)
	}
	if state != "ACTIVE" || observed != "RUNNING" || name != "LWSテストアプリ" || service != "web" {
		t.Fatalf("登録後の状態が不正です: state=%s observed=%s name=%s service=%s", state, observed, name, service)
	}
	if !runner.hasDockerComposeUp() {
		t.Fatal("登録後にDocker Compose upが実行されていません")
	}
	caddy, err := os.ReadFile(filepath.Join(executor.Derived.GeneratedDir, "Caddyfile"))
	if err != nil || !strings.Contains(string(caddy), "valid.example.internal") {
		t.Fatalf("Caddyfileが生成されていません: %v\n%s", err, caddy)
	}
	hosts, err := os.ReadFile(filepath.Join(executor.Derived.GeneratedDir, "hosts"))
	if err != nil || !strings.Contains(string(hosts), "valid.example.internal") {
		t.Fatalf("hostsが生成されていません: %v\n%s", err, hosts)
	}
}

func TestFTV1_023InvalidManifestDoesNotStartDocker(t *testing.T) {
	url := prepareFeatureRepository(t, "invalid-manifest", "lws-invalid-manifest")
	runner := &featureRunner{}
	db, _, server := newFeatureServer(t, runner)

	body := strings.NewReader(fmt.Sprintf(`{"repositoryUrl":%q,"ref":"main","subdomain":"invalid-manifest","requestId":"550e8400-e29b-41d4-a716-446655440202"}`, url))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/applications", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("登録受付に失敗しました: status=%d body=%s", response.Code, response.Body.String())
	}
	var accepted map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	result := waitFeatureOperation(t, db, accepted["name"])
	if !strings.HasPrefix(result, "FAILED\x00") {
		t.Fatalf("不正manifestを成功扱いしました: %q", result)
	}
	if runner.hasDockerComposeUp() {
		t.Fatal("manifest検証失敗後にDocker Compose upが実行されました")
	}
}

func TestFTV1_026GitCloneFailureKeepsApplicationUnchanged(t *testing.T) {
	runner := &featureRunner{gitError: true}
	db, _, server := newFeatureServer(t, runner)
	url := "https://github.com/test/lws-git-failure"
	body := strings.NewReader(fmt.Sprintf(`{"repositoryUrl":%q,"ref":"main","subdomain":"git-failure","requestId":"550e8400-e29b-41d4-a716-446655440203"}`, url))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/applications", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("登録受付に失敗しました: status=%d body=%s", response.Code, response.Body.String())
	}
	var accepted map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	result := waitFeatureOperation(t, db, accepted["name"])
	if !strings.HasPrefix(result, "FAILED\x00") {
		t.Fatalf("Git失敗を成功扱いしました: %q", result)
	}
	var state string
	if err := db.QueryRow(`SELECT registration_state FROM applications WHERE subdomain='git-failure'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "ACTIVE" {
		t.Fatalf("Git失敗後の登録状態が不正です: %s", state)
	}
	if runner.hasDockerComposeUp() {
		t.Fatal("Git取得失敗後にDocker Compose upが実行されました")
	}
}
