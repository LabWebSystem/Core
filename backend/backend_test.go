package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDBInitializesWithWALAndForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := OpenDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fk, wal, err := sqlitePragmas(context.Background(), db)
	if err != nil || !fk || !wal {
		t.Fatalf("pragmas fk=%v wal=%v err=%v", fk, wal, err)
	}
	var applied int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != 2 {
		t.Fatalf("migration count=%d err=%v", applied, err)
	}
	db.Close()
	db, err = OpenDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != 2 {
		t.Fatalf("migration rerun count=%d err=%v", applied, err)
	}
}

func TestOpenAPIContractAndGeneratedRoutes(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	if spec.OpenAPI != "3.1.0" || spec.Paths == nil || spec.Paths.Find("/applications") == nil {
		t.Fatalf("invalid generated OpenAPI contract: version=%s", spec.OpenAPI)
	}
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/applications", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "applications") {
		t.Fatalf("generated route status=%d body=%s", rr.Code, rr.Body.String())
	}
}
func TestValidateRepositoryURL(t *testing.T) {
	for _, good := range []string{"https://github.com/a/b", "https://github.com/a/b.git"} {
		if err := ValidateRepositoryURL(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{"git@github.com:a/b", "https://gitlab.com/a/b", "https://github.com/a/b?x=y", "https://user:pass@github.com/a/b"} {
		if err := ValidateRepositoryURL(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestValidateInstallationID(t *testing.T) {
	if err := ValidateInstallationID("installation-1"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "  ", "bad\nvalue"} {
		if err := ValidateInstallationID(value); err == nil {
			t.Fatalf("invalid installation ID accepted: %q", value)
		}
	}
}
func TestValidateManifest(t *testing.T) {
	m, err := ValidateManifest([]byte("apiVersion: lws/v1\nmetadata:\n  name: Demo\n  description: test\npublic:\n  service: web\n  port: 3000\n"))
	if err != nil || m.Public.Port != 3000 {
		t.Fatal(m, err)
	}
	_, err = ValidateManifest([]byte("apiVersion: wrong\nmetadata: {}\npublic: {}\n"))
	if err == nil {
		t.Fatal("invalid manifest accepted")
	}
	for _, bad := range []string{
		"apiVersion: lws/v1\nmetadata:\n  name: Demo\n  name: Again\npublic:\n  service: web\n  port: 3000\n",
		"apiVersion: lws/v1\nmetadata:\n  name: Demo\n  labels: x\npublic:\n  service: web\n  port: 3000\n",
		"apiVersion: lws/v1\nmetadata:\n  name: Demo\npublic:\n  service: web\n  port: \"3000\"\n",
		"apiVersion: lws/v1\nmetadata: &m\n  name: Demo\npublic: *m\n",
	} {
		if _, err := ValidateManifest([]byte(bad)); err == nil {
			t.Fatalf("invalid manifest accepted: %s", bad)
		}
	}
	owned := []byte(`{"services":{"web":{"networks":{"lws-edge":{}}}},"networks":{"lws-edge":{"external":true,"name":"lws-app-id-edge"}}}`)
	if err := ValidateEffectiveComposeWithOwnedNetwork(owned, "web", 3000, "lws-app-id-edge"); err != nil {
		t.Fatalf("owned edge network rejected: %v", err)
	}
	if err := ValidateEffectiveComposeWithOwnedNetwork(owned, "web", 3000, "lws-other-edge"); err == nil {
		t.Fatal("foreign edge network accepted")
	}
}
func TestValidateProjectPath(t *testing.T) {
	if err := ValidateProjectPath("/tmp/project", "../../etc"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := ValidateProjectPath("/tmp/project", "./src"); err != nil {
		t.Fatal(err)
	}
}

func TestExtractComposeVariables(t *testing.T) {
	variables, err := ExtractComposeVariables([]byte("services:\n  web:\n    image: ${IMAGE:?IMAGEが必要}\n    environment:\n      PORT: ${PORT:-3000}\n      TOKEN: ${TOKEN}\n      LABEL: $LABEL\n"))
	if err != nil || len(variables) != 4 {
		t.Fatalf("variables=%+v err=%v", variables, err)
	}
	if !variables[0].Required || variables[0].Name != "IMAGE" || !variables[2].HasDefault || variables[2].Name != "PORT" {
		t.Fatalf("unexpected variables=%+v", variables)
	}
	if err := ValidateComposeVariableValues(variables, map[string]string{"IMAGE": "example", "TOKEN": "secret", "LABEL": "label"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateComposeVariableValues(variables, map[string]string{"TOKEN": "secret"}); err == nil {
		t.Fatal("missing required variable accepted")
	}
	for _, bad := range []string{"${lower}", "${BROKEN", "${A!}"} {
		if _, err := ExtractComposeVariables([]byte(bad)); err == nil {
			t.Fatalf("invalid variable accepted: %s", bad)
		}
	}
}
func TestHTTPHealth(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health/live", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health/ready", nil))
	if rr.Code != 503 {
		t.Fatal(rr.Code)
	}
}
func TestHTTPRejectsOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "/health/live", nil)
	r.Header.Set("Origin", "https://bad")
	rr := httptest.NewRecorder()
	r.URL.Path = "/api/v1/health/live"
	(&Server{}).Handler().ServeHTTP(rr, r)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "ORIGIN_FORBIDDEN") {
		t.Fatal(rr.Code, rr.Body.String())
	}
}

func TestHTTPRejectsUnexpectedHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	r.Host = "bad.example.internal"
	rr := httptest.NewRecorder()
	(&Server{AllowedHost: "api.example.internal"}).Handler().ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "HOST_FORBIDDEN") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPReadyRejectsClosedDatabase(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHTTPStateChangeRejectsClosedDatabase(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/applications", strings.NewReader(`{"repositoryUrl":"https://github.com/a/b","ref":"main","subdomain":"app","requestId":"550e8400-e29b-41d4-a716-446655440000"}`))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, r)
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "DATABASE_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPRejectsInvalidRepositoryBeforePersistence(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/applications", strings.NewReader(`{"repositoryUrl":"https://gitlab.com/a/b","ref":"main","subdomain":"app","requestId":"550e8400-e29b-41d4-a716-446655440000"}`))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, r)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "INVALID_REPOSITORY_URL") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count); err != nil || count != 0 {
		t.Fatalf("applications count=%d err=%v", count, err)
	}
}

func TestHTTPRejectsConcurrentApplicationOperation(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOperation(context.Background(), db, "a", "550e8400-e29b-41d4-a716-446655440010", "START"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/applications/a:stop", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-Id", "550e8400-e29b-41d4-a716-446655440011")
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, r)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "CONFLICT") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
func TestManifestSymlinkIsNotUsed(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "manifest")
	if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(p, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceTree(d); err == nil {
		t.Fatal("source symlink accepted")
	}
}

func TestSourceTreeRejectsDotEnv(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, ".env"), []byte("TOKEN=secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceTree(d); err == nil {
		t.Fatal("source .env accepted")
	}
}

func TestRestoreSourceSwapKeepsPreviousSource(t *testing.T) {
	d := t.TempDir()
	current := filepath.Join(d, "source")
	old := current + ".old"
	if err := os.MkdirAll(current, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "version"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "version"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceSwap(current); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(current, "version"))
	if err != nil || string(data) != "old" {
		t.Fatalf("restored source=%q err=%v", data, err)
	}
}

func TestRuntimeRestoresSourceWhenComposeValidationFails(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "apps")
	source := filepath.Join(root, "app-id", "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "marker"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	r := &cloneValidationRunner{}
	e := &RuntimeExecutor{DB: db, Root: root, Runner: r}
	if err := e.Run(context.Background(), Operation{ApplicationID: "app-id", Kind: "CREATE"}); err == nil {
		t.Fatal("forbidden Compose was accepted")
	}
	marker, err := os.ReadFile(filepath.Join(source, "marker"))
	if err != nil || string(marker) != "old" {
		t.Fatalf("old source was not restored: %q err=%v", marker, err)
	}
}

type cloneValidationRunner struct{}

func (r *cloneValidationRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "git" {
		dest := args[len(args)-1]
		if err := os.MkdirAll(dest, 0700); err != nil {
			return nil, err
		}
		manifest := "apiVersion: lws/v1\nmetadata:\n  name: New\n  description: new\npublic:\n  service: web\n  port: 3000\n"
		if err := os.WriteFile(filepath.Join(dest, "lws.manifest.yaml"), []byte(manifest), 0600); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dest, "compose.yaml"), []byte("services:\n  web:\n    image: example\n"), 0600); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return []byte(`{"services":{"web":{"volumes":["./host:/data"]}}}`), nil
}

func TestComposeRejectsExternalFeatures(t *testing.T) {
	err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    env_file: .env\n"))
	if err == nil {
		t.Fatal("env_file accepted")
	}
	if err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    build:\n      context: ../../outside\n")); err == nil {
		t.Fatal("outside context accepted")
	}
	if err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    image: one\n    image: two\n")); err == nil {
		t.Fatal("duplicate compose key accepted")
	}
	if err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    build:\n      context: https://example.com/source.git\n")); err == nil {
		t.Fatal("remote build context accepted")
	}
	if err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    configs:\n      - source: config\n")); err == nil {
		t.Fatal("file configs accepted")
	}
}

func TestEffectiveComposeAllowsNamedVolumeButRejectsBindMount(t *testing.T) {
	named := []byte(`{"services":{"web":{"volumes":["app-data:/data"]}}}`)
	if err := ValidateEffectiveCompose(named, "web", 3000); err != nil {
		t.Fatalf("named volume rejected: %v", err)
	}
	bind := []byte(`{"services":{"web":{"volumes":["./data:/data"]}}}`)
	if err := ValidateEffectiveCompose(bind, "web", 3000); err == nil {
		t.Fatal("bind mount accepted")
	}
	for _, bad := range []string{
		`{"services":{"web":{"volumes":["/data"]}}}`,
		`{"services":{"web":{"tmpfs":["/tmp"]}}}`,
		`{"services":{"web":{"image":"x"}},"networks":{"outside":{"external":true}}}`,
	} {
		if err := ValidateEffectiveCompose([]byte(bad), "web", 3000); err == nil {
			t.Fatalf("forbidden effective compose accepted: %s", bad)
		}
	}
}

func TestOperationRequestIDIdempotency(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requestID := "550e8400-e29b-41d4-a716-446655440000"
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	a, err := CreateOperation(context.Background(), db, "a", requestID, "START")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateOperation(context.Background(), db, "a", requestID, "START")
	if err != nil || a.ID != b.ID {
		t.Fatal(a, b, err)
	}
	if _, err := CreateOperation(context.Background(), db, "a", "550e8400-e29b-41d4-a716-446655440001", "STOP"); err == nil {
		t.Fatal("同一appの競合Operationを受け付けました")
	}
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('b','app-b','https://github.com/a/c','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOperationWithFingerprint(context.Background(), db, "b", "550e8400-e29b-41d4-a716-446655440002", "CREATE", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOperationWithFingerprint(context.Background(), db, "b", "550e8400-e29b-41d4-a716-446655440002", "CREATE", "second"); err == nil {
		t.Fatal("異なる内容のrequestId再利用を受け付けました")
	}
}

func TestWorkerSerializesAppsAndLimitsParallelism(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var active, maximum atomic.Int32
	started := make(chan string, 3)
	release := make(chan struct{})
	w := NewWorker(db, func(_ context.Context, op Operation) error {
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- op.ApplicationID
		<-release
		active.Add(-1)
		return nil
	})
	w.Enqueue(Operation{ID: "1", ApplicationID: "a"})
	w.Enqueue(Operation{ID: "2", ApplicationID: "b"})
	w.Enqueue(Operation{ID: "3", ApplicationID: "c"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("workerが開始されませんでした")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("2つ目のworkerが開始されませんでした")
	}
	select {
	case app := <-started:
		t.Fatalf("上限を超えてworkerが開始されました: %s", app)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if maximum.Load() != 2 {
		t.Fatalf("最大並列数=%d", maximum.Load())
	}
	serial := NewWorker(db, func(_ context.Context, _ Operation) error { return nil })
	if err := serial.Enqueue(Operation{ID: "4", ApplicationID: "same"}); err != nil {
		t.Fatal(err)
	}
	if err := serial.Enqueue(Operation{ID: "5", ApplicationID: "same"}); err == nil {
		t.Fatal("同一appの競合workerを受け付けました")
	}
}

func TestSecretConfigurationIsEncryptedAndNeverReturned(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	s := &Server{DB: db, SecretKey: key}
	body := `{"variables":{"TOKEN":{"value":"secret-value","secret":true}},"requestId":"550e8400-e29b-41d4-a716-446655440000"}`
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/applications/a/configuration", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, r)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var stored []byte
	if err := db.QueryRow(`SELECT value FROM application_variables WHERE application_id='a' AND name='TOKEN'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == "secret-value" {
		t.Fatal("secret was stored as plaintext")
	}
	plain, err := Decrypt(key, stored)
	if err != nil || string(plain) != "secret-value" {
		t.Fatalf("decrypt secret: %q, %v", plain, err)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/applications/a/configuration", nil)
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, r)
	if strings.Contains(rr.Body.String(), "secret-value") {
		t.Fatal("secret was returned")
	}
}

func TestDerivedConfigIsAtomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	if err := WriteAtomic(p, []byte(GenerateHosts("example.internal", "192.0.2.1", []PublishedApplication{{Subdomain: "app", AppID: "id", Service: "web", Port: 3000}})), 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "app.example.internal") {
		t.Fatal(string(data))
	}
}

type recordingRunner struct {
	name   string
	args   []string
	output []byte
	calls  [][]string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = args
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.output, nil
}
func TestRuntimeUsesOwnedComposeArguments(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	runtime := filepath.Join(dir, "app-id", "runtime")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "compose.yaml"), []byte("services:\n  web:\n    image: example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "lws.manifest.yaml"), []byte("apiVersion: lws/v1\nmetadata:\n  name: App\n  description: test\npublic:\n  service: web\n  port: 3000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &recordingRunner{output: []byte(`{"services":{"web":{"image":"example"}}}`)}
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := &RuntimeExecutor{DB: db, Root: dir, Runner: r}
	if err := e.reconcile(context.Background(), "app-id", source, runtime, "up", "-d"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(r.args, " ")
	if !strings.Contains(got, "--project-name lws-app-app-id") || !strings.Contains(got, "--env-file "+filepath.Join(runtime, "app.env")) || !strings.Contains(got, "-f "+filepath.Join(source, "compose.yaml")) || !strings.Contains(got, "-f "+filepath.Join(runtime, "lws.override.yaml")) || strings.Contains(got, "compose.override.yaml") || strings.Contains(got, "--volumes") {
		t.Fatal(got)
	}
}

func TestOSRunnerUsesArgvAndTimeout(t *testing.T) {
	started := time.Now()
	_, err := (OSRunner{Timeout: 25 * time.Millisecond}).Run(context.Background(), "sh", "-c", "sleep 1")
	if err == nil {
		t.Fatal("timeoutなしでコマンドが完了しました")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("timeoutが効いていません: %s", elapsed)
	}
}

func TestGeneratedOverrideCarriesOwnershipLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.yaml")
	e := &RuntimeExecutor{InstallationID: "installation"}
	if err := e.GenerateOverride("app-id", "web", path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"com.labwebsystem.owner", "com.labwebsystem.installation-id", "com.labwebsystem.app-id", "app-id"} {
		if !strings.Contains(text, want) {
			t.Fatalf("override lacks %q: %s", want, text)
		}
	}
}

func TestNamedVolumesReceiveOwnershipLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.yaml")
	e := &RuntimeExecutor{InstallationID: "installation"}
	if err := e.GenerateOverrideWithVolumes("app-id", "web", []string{"data"}, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "data") || !strings.Contains(string(data), "com.labwebsystem.app-id") {
		t.Fatalf("volume ownership labels missing: %s", data)
	}
}

func TestRuntimeWritesValidatedEnvironmentFile(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	runtime := filepath.Join(dir, "app-id", "runtime")
	if err := os.WriteFile(compose, []byte("services:\n  web:\n    image: ${IMAGE:?IMAGEが必要}\n    environment:\n      TOKEN: ${TOKEN}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	key := []byte("01234567890123456789012345678901")
	secret, err := Encrypt(key, []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO application_variables(application_id,name,value,is_secret,updated_at) VALUES ('app-id','TOKEN',?,1,datetime('now'))`, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO application_variables(application_id,name,value,is_secret,updated_at) VALUES ('app-id','IMAGE','example',0,datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	e := &RuntimeExecutor{DB: db, SecretKey: key}
	if err := e.prepareEnvironment(context.Background(), compose, runtime); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runtime, "app.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "TOKEN=\"secret-value\"") || !strings.Contains(string(data), "IMAGE=\"example\"") {
		t.Fatalf("invalid app.env: %s", data)
	}
	if mode := func() os.FileMode { info, _ := os.Stat(filepath.Join(runtime, "app.env")); return info.Mode().Perm() }(); mode != 0600 {
		t.Fatalf("app.env mode=%o", mode)
	}
}

func TestRuntimeRejectsEffectiveComposeBeforeUp(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	runtime := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "compose.yaml"), []byte("services:\n  web:\n    image: example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "lws.manifest.yaml"), []byte("apiVersion: lws/v1\nmetadata:\n  name: App\n  description: test\npublic:\n  service: web\n  port: 3000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &recordingRunner{output: []byte(`{"services":{"web":{"volumes":["./data:/data"]}}}`)}
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := &RuntimeExecutor{DB: db, Root: dir, Runner: r}
	err = e.reconcile(context.Background(), "app-id", source, runtime, "up", "-d")
	if err == nil {
		t.Fatal("forbidden effective Compose accepted")
	}
	if len(r.calls) != 1 || !strings.Contains(strings.Join(r.calls[0], " "), "config --format json") {
		t.Fatalf("unexpected commands: %v", r.calls)
	}
}

func TestDockerResourceNeverRemovesForeignNetwork(t *testing.T) {
	r := &recordingRunner{output: []byte(`[{"Id":"n1","Name":"foreign","Labels":{"com.labwebsystem.owner":"other"}}]`)}
	d := NewDockerResources(r, "installation")
	if err := d.RemoveNetwork(context.Background(), "app", "foreign"); err == nil {
		t.Fatal("外部networkを削除できました")
	}
	if strings.Contains(strings.Join(r.args, " "), "network rm") {
		t.Fatal("外部networkへの削除が発行されました")
	}
}

func TestDockerNetworkDoesNotCreateAfterInspectFailure(t *testing.T) {
	r := &errorRunner{output: []byte("daemon is unavailable")}
	d := NewDockerResources(r, "installation")
	if err := d.EnsureNetwork(context.Background(), "app"); err == nil {
		t.Fatal("daemon failure was treated as resource absence")
	}
	if len(r.calls) != 1 {
		t.Fatalf("unexpected Docker calls: %v", r.calls)
	}
}

func TestDockerConnectsCaddyWithAppAlias(t *testing.T) {
	r := &recordingRunner{}
	d := NewDockerResources(r, "installation")
	d.CaddyContainer = "caddy-container"
	if err := d.EnsureCaddyConnected(context.Background(), "app-id"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(r.args, " ")
	if !strings.Contains(got, "network connect --alias lws-app-id lws-app-app-id-edge caddy-container") {
		t.Fatalf("unexpected command: %s", got)
	}
}

func TestDockerDisconnectsCaddyIdempotently(t *testing.T) {
	r := &failingCommandRunner{err: fmt.Errorf("not connected")}
	d := NewDockerResources(r, "installation")
	if err := d.DisconnectCaddy(context.Background(), "app-id"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.args, " "), "network disconnect -f lws-app-app-id-edge lws-caddy-1") {
		t.Fatalf("unexpected command: %v", r.args)
	}
}

func TestDockerProjectRejectsForeignContainer(t *testing.T) {
	r := &sequenceRunner{outputs: [][]byte{
		[]byte("c1\n"),
		[]byte(`[{"Id":"c1","Config":{"Labels":{"com.labwebsystem.owner":"other"}}}]`),
	}}
	d := NewDockerResources(r, "installation")
	if err := d.VerifyProjectOwnership(context.Background(), "app-id"); err == nil {
		t.Fatal("foreign project container accepted")
	}
	if len(r.calls) != 2 {
		t.Fatalf("unexpected Docker calls: %v", r.calls)
	}
}

func TestDockerPurgeRemovesOnlyOwnedVolumes(t *testing.T) {
	r := &sequenceRunner{outputs: [][]byte{
		[]byte("owned-volume\n"),
		[]byte(`[{"Name":"owned-volume","Labels":{"com.labwebsystem.owner":"lws","com.labwebsystem.installation-id":"installation","com.labwebsystem.app-id":"app-id"}}]`),
		[]byte{},
	}}
	d := NewDockerResources(r, "installation")
	if err := d.RemoveOwnedVolumes(context.Background(), "app-id"); err != nil {
		t.Fatal(err)
	}
	joined := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		joined = append(joined, strings.Join(call, " "))
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "volume rm owned-volume") || strings.Contains(all, "system prune") || strings.Contains(all, "volume prune") || strings.Contains(all, "--volumes") {
		t.Fatalf("不正なvolume操作: %s", all)
	}
}

type errorRunner struct {
	output []byte
	calls  [][]string
}

func (r *errorRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.output, fmt.Errorf("daemon failure")
}

type failingCommandRunner struct {
	args []string
	err  error
}

func (r *failingCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return nil, r.err
}

type sequenceRunner struct {
	outputs [][]byte
	calls   [][]string
}

func (r *sequenceRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	output := r.outputs[0]
	r.outputs = r.outputs[1:]
	return output, nil
}
