package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestGetApplicationIncludesObservedAt(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	NewServer(db, nil).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/applications/app", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "observedAt") {
		t.Fatalf("observedAtがありません: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestListApplicationsReturnsApplicationContract(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	NewServer(db, nil).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/applications", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Applications []map[string]any `json:"applications"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Applications) != 1 {
		t.Fatalf("一覧件数=%d", len(response.Applications))
	}
	for _, field := range []string{"name", "subdomain", "repositoryUrl", "ref", "desiredState", "observedState", "registrationState", "observedAt", "reconciling", "etag"} {
		if _, ok := response.Applications[0][field]; !ok {
			t.Fatalf("一覧Applicationの必須項目がありません: %s", field)
		}
	}
}

func TestUnknownApplicationOperationIsRejected(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/app:unknown", strings.NewReader(`{"requestId":"550e8400-e29b-41d4-a716-446655440114"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	NewServer(db, nil).Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("未定義operationを受理しました: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestConfigurationRequiresExistingApplication(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rr := httptest.NewRecorder()
	NewServer(db, nil).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/applications/missing/configuration", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("存在しないアプリの設定を返しました: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestInfrastructureImagesArePinnedAndRequiredPortsAreFixed(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "infrastructure", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, image := range []string{"lws-backend:", "lws-dashboard:", "caddy:", "coredns/coredns:"} {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, image) && strings.Contains(line, "@") && strings.Contains(line, "sha256:") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("固定digestのimageがありません: %s", image)
		}
	}
	for _, port := range []string{"\"${LWS_HTTP_PORT:-80}:80\"", "\"${LWS_DNS_PORT:-53}:53/tcp\"", "\"${LWS_DNS_PORT:-53}:53/udp\""} {
		if !strings.Contains(text, port) {
			t.Fatalf("必須公開portが固定されていません: %s", port)
		}
	}
	for _, variable := range []string{"LWS_BACKEND_DIGEST", "LWS_DASHBOARD_DIGEST"} {
		if !strings.Contains(text, variable+":-") {
			t.Fatalf("リリースdigestの注入点がありません: %s", variable)
		}
	}
}

func TestOpenAPIDeclaresResponseSchemasAndObservedAt(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/applications", "/applications/{application}", "/operations/{operation}"} {
		item := spec.Paths.Find(path)
		if item == nil {
			t.Fatalf("OpenAPI pathがありません: %s", path)
		}
		for _, operation := range []*openapi3.Operation{item.Get, item.Post, item.Patch, item.Delete} {
			if operation == nil {
				continue
			}
			response := operation.Responses.Value("200")
			if response == nil {
				response = operation.Responses.Value("202")
			}
			if response == nil || response.Value == nil || response.Value.Content == nil {
				t.Fatalf("response schemaがありません: %s", path)
			}
		}
	}
	application := spec.Components.Schemas["Application"]
	if application == nil || application.Value.Properties["observedAt"] == nil {
		t.Fatal("Application.observedAtがOpenAPIにありません")
	}
}

func TestCreateApplicationRollsBackWhenOperationCreationFails(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_operation BEFORE INSERT ON operations BEGIN SELECT RAISE(ABORT, 'operation unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	body := strings.NewReader(`{"repositoryUrl":"https://github.com/a/b","ref":"main","subdomain":"demo","requestId":"550e8400-e29b-41d4-a716-446655440113"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications", body)
	req.Header.Set("Content-Type", "application/json")
	NewServer(db, nil).Handler().ServeHTTP(rr, req)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM applications WHERE subdomain='demo'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("Operation作成失敗時にアプリが残りました")
	}
}

func TestDockerTailLogsUsesComposeFilesAndRedactsSecret(t *testing.T) {
	runner := &streamTestRunner{inspect: `[ {"Id":"container","Config":{"Labels":{"com.labwebsystem.owner":"lws","com.labwebsystem.installation-id":"installation","com.docker.compose.project":"lws-app-id"}}} ]`}
	docker := NewDockerResources(runner, "installation")
	lines, err := docker.TailLogs(context.Background(), "id", "/tmp/app.env", "/tmp/compose.yaml", "/tmp/override.yaml", []string{"secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	line := <-lines
	if strings.Contains(line, "secret-value") || line != "token=[REDACTED]" {
		t.Fatalf("ログのsecretが除去されていません: %q", line)
	}
	joined := strings.Join(runner.args, " ")
	for _, expected := range []string{"--env-file /tmp/app.env", "-f /tmp/compose.yaml", "-f /tmp/override.yaml", "logs --follow"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("ログ取得引数が不足しています: %s", joined)
		}
	}
}

type streamTestRunner struct {
	args    []string
	inspect string
}

func (r *streamTestRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "docker" && len(args) >= 3 && args[1] == "ps" {
		r.args = append(r.args, args...)
		return []byte("container\n"), nil
	}
	if name == "docker" && len(args) >= 3 && args[1] == "container" && args[2] == "inspect" {
		return []byte(r.inspect), nil
	}
	return nil, nil
}

func (r *streamTestRunner) Stream(_ context.Context, name string, args ...string) (io.ReadCloser, error) {
	r.args = append(r.args, args...)
	return io.NopCloser(strings.NewReader("token=secret-value\n")), nil
}

func TestOperationSSEPublishesEnvelopeAndReplaysTerminalState(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	op, err := CreateOperation(context.Background(), db, "app", "550e8400-e29b-41d4-a716-446655440111", "START")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetOperationState(context.Background(), db, op.ID, "SUCCEEDED", ""); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(NewServer(db, nil).Handler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/v1/operations/" + op.ID + ":watch")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "event: succeeded") || !strings.Contains(text, "sequence") || !strings.Contains(text, "timestamp") || !strings.Contains(text, `"type":"succeeded"`) {
		t.Fatalf("SSE envelopeが不足しています: %s", text)
	}
}

func TestOperationSSEDisconnectAndReconnectCanRecoverFromHTTP(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	op, err := CreateOperation(context.Background(), db, "app", "550e8400-e29b-41d4-a716-446655440112", "START")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetOperationState(context.Background(), db, op.ID, "FAILED", "失敗"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(db, nil).Handler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/v1/operations/" + op.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var got map[string]any
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["state"] != "failed" {
		t.Fatalf("HTTP再取得で状態を回復できません: %#v", got)
	}
}

func TestSSESlowSubscriberIsBoundedAndDoesNotBlockPublisher(t *testing.T) {
	events := NewEvents()
	sub := events.Subscribe("operation")
	for i := 0; i < 100; i++ {
		done := make(chan struct{})
		go func(i int) {
			events.Publish("operation", "log", map[string]int{"line": i})
			close(done)
		}(i)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("低速購読者がイベント発行を停止させました")
		}
	}
	sub.Close()
}

func TestContainerLogsAreStreamedAsSSE(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	logs := make(chan string, 1)
	serverImpl := NewServer(db, nil)
	serverImpl.LogSource = func(context.Context, string) (<-chan string, error) { return logs, nil }
	httpServer := httptest.NewServer(serverImpl.Handler())
	defer httpServer.Close()
	response, err := httpServer.Client().Get(httpServer.URL + "/api/v1/applications/app:tailLogs")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("ログSSEではありません: %q body=%s", response.Header.Get("Content-Type"), body)
	}
	logs <- "secretを含まないログ行"
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != ": connected\n" {
		t.Fatalf("接続eventがありません: %q err=%v", line, err)
	}
	line, err = reader.ReadString('\n')
	if line == "\n" {
		line, err = reader.ReadString('\n')
	}
	if err != nil || !strings.HasPrefix(line, "id: ") {
		t.Fatalf("ログevent idがありません: %q err=%v", line, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRestartMarksUnfinishedOperationsFailed(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"app-a", "app-b"} {
		if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES (?, ?, 'https://github.com/a/b','main',datetime('now'),datetime('now'))`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	for i, state := range []string{"QUEUED", "RUNNING"} {
		op, err := CreateOperation(context.Background(), db, "app-"+string(rune('a'+i)), "550e8400-e29b-41d4-a716-44665544012"+string(rune('0'+i)), "START")
		if err != nil {
			t.Fatal(err)
		}
		if err := SetOperationState(context.Background(), db, op.ID, state, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := MarkUnfinishedOperations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM operations WHERE state='FAILED' AND error_message LIKE '%再起動%'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("未完了Operationが整理されていません: count=%d err=%v", count, err)
	}
}

func TestLoadSecretKeyRejectsInsecureExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(path, make([]byte, 32), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSecretKey(path); err == nil {
		t.Fatal("権限の弱いsecret keyを受理しました")
	}
}

func TestLoadSecretKeyRejectsCorruptExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(path, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSecretKey(path); err == nil {
		t.Fatal("破損したsecret keyを受理しました")
	}
}

func TestLoadSecretKeyRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "secret.key")
	if err := os.WriteFile(target, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSecretKey(path); err == nil {
		t.Fatal("symlinkのsecret keyを受理しました")
	}
}

func TestLoadSecretKeyCreatesPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "secret.key")
	key, err := LoadSecretKey(path)
	if err != nil || len(key) != 32 {
		t.Fatalf("secret keyを作成できません: len=%d err=%v", len(key), err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("secret keyの権限が0600ではありません: %o", info.Mode().Perm())
	}
}
