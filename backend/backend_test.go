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
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != 5 {
		t.Fatalf("migration count=%d err=%v", applied, err)
	}
	db.Close()
	db, err = OpenDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != 5 {
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

func TestValidateRequestIDRequiresUUIDv4(t *testing.T) {
	if err := ValidateRequestID("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestID("550e8400-e29b-11d4-a716-446655440000"); err == nil {
		t.Fatal("UUID v4以外のrequestIdを受け付けました")
	}
}

func TestValidateBaseDomain(t *testing.T) {
	for _, value := range []string{"example.internal", "lab.example.internal"} {
		if err := ValidateBaseDomain(value); err != nil {
			t.Errorf("有効なbase domainを拒否しました: %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "example", ".example.internal", "example.internal.", "https://example.internal", "bad domain", "example..internal"} {
		if err := ValidateBaseDomain(value); err == nil {
			t.Errorf("不正なbase domainを受理しました: %q", value)
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

func TestHTTPAcceptsDashboardOriginOnly(t *testing.T) {
	s := &Server{AllowedHost: "api.example.internal", AllowedOrigin: "http://dashboard.example.internal"}
	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	request.Host = "api.example.internal"
	request.Header.Set("Origin", "http://dashboard.example.internal")
	s.Handler().ServeHTTP(allowed, request)
	if allowed.Code != http.StatusOK {
		t.Fatalf("dashboard origin status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	rejected := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	request.Host = "api.example.internal"
	request.Header.Set("Origin", "http://other.example.internal")
	s.Handler().ServeHTTP(rejected, request)
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("unexpected origin status=%d", rejected.Code)
	}
}

func TestDerivedDashboardRoute(t *testing.T) {
	hosts := GenerateHosts("example.internal", "192.0.2.1", nil)
	if !strings.Contains(hosts, "192.0.2.1 dashboard.example.internal\n") {
		t.Fatalf("dashboard host is missing: %s", hosts)
	}
	caddy := GenerateCaddyfile("example.internal", nil)
	for _, want := range []string{"http://dashboard.example.internal", "handle /api/*", "reverse_proxy dashboard:80", "header_up Host api.example.internal"} {
		if !strings.Contains(caddy, want) {
			t.Fatalf("dashboard route is missing %q: %s", want, caddy)
		}
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

func TestHTTPUsesOpenAPISchemaValidationBeforeHandler(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/applications", strings.NewReader(`{"repositoryUrl":"https://github.com/a/b","subdomain":"app","requestId":"550e8400-e29b-41d4-a716-446655440000"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("OpenAPI schema違反を受理しました: status=%d body=%s", w.Code, w.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("schema違反でDB副作用が発生しました: count=%d err=%v", count, err)
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

func TestHTTPReadyHonorsRuntimeReadiness(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rr := httptest.NewRecorder()
	(&Server{DB: db, Ready: func() bool { return false }}).Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/health/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("runtime未準備状態をreadyとして返しました: %d", rr.Code)
	}
}

func TestHTTPReadEndpointsRejectMissingDatabase(t *testing.T) {
	for _, path := range []string{"/api/v1/applications/app", "/api/v1/operations/op", "/api/v1/applications/app/configuration"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			(&Server{}).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("DBなしのreadを受理しました: path=%s status=%d body=%s", path, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHTTPStateChangingEndpointsRejectMissingDatabase(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{name: "update", path: "/api/v1/applications/app", body: `{"requestId":"550e8400-e29b-41d4-a716-446655440115","ref":"main"}`},
		{name: "start", path: "/api/v1/applications/app:start", body: `{"requestId":"550e8400-e29b-41d4-a716-446655440116"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, tc.path, strings.NewReader(tc.body))
			if tc.name == "start" {
				req.Method = http.MethodPost
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			(&Server{}).Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("DBなしのstate changeを受理しました: status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
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
	r := httptest.NewRequest(http.MethodPost, "/api/v1/applications/a:stop", strings.NewReader(`{"requestId":"550e8400-e29b-41d4-a716-446655440011"}`))
	r.Header.Set("Content-Type", "application/json")
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

func TestRuntimeUpdateRestoresApplicationOnSourceFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app-id','old','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	payload := `{"ref":"feature","subdomain":"new"}`
	e := &RuntimeExecutor{DB: db, Root: dir, Runner: &failingCommandRunner{err: fmt.Errorf("git failure")}}
	err = e.Run(context.Background(), Operation{ApplicationID: "app-id", Kind: "UPDATE", Payload: payload})
	if err == nil {
		t.Fatal("source failureを成功扱いしました")
	}
	var subdomain, ref string
	if err := db.QueryRow(`SELECT subdomain,git_ref FROM applications WHERE id='app-id'`).Scan(&subdomain, &ref); err != nil {
		t.Fatal(err)
	}
	if subdomain != "old" || ref != "main" {
		t.Fatalf("更新前のアプリ情報を復元できません: subdomain=%s ref=%s", subdomain, ref)
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

func TestConfigurationRequestIDRejectsDifferentContentWithoutSideEffect(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	server := (&Server{DB: db, SecretKey: []byte("01234567890123456789012345678901")})
	requestID := "550e8400-e29b-41d4-a716-446655440099"
	patch := func(value string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"variables":{"TOKEN":{"value":%q,"secret":false}},"requestId":%q}`, value, requestID)
		r := httptest.NewRequest(http.MethodPatch, "/api/v1/applications/a/configuration", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, r)
		return rr
	}
	if rr := patch("first"); rr.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := patch("second"); rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "REQUEST_ID_REUSED") {
		t.Fatalf("replay status=%d body=%s", rr.Code, rr.Body.String())
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM application_variables WHERE application_id='a' AND name='TOKEN'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "first" {
		t.Fatalf("異なるrequestId内容で設定が変更されました: %s", value)
	}
}

func TestConfigurationConflictHasNoSideEffect(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOperation(context.Background(), db, "a", "550e8400-e29b-41d4-a716-446655440098", "START"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/applications/a/configuration", strings.NewReader(`{"variables":{"TOKEN":{"value":"must-not-save","secret":false}},"requestId":"550e8400-e29b-41d4-a716-446655440097"}`))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, r)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM application_variables WHERE application_id='a'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("競合要求で設定が保存されました: count=%d err=%v", count, err)
	}
}

func TestGetApplicationIncludesOperationStateFields(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOperation(context.Background(), db, "a", "550e8400-e29b-41d4-a716-446655440096", "START"); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/applications/a", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "latestOperation") || !strings.Contains(rr.Body.String(), "reconciling") || !strings.Contains(rr.Body.String(), "etag") {
		t.Fatalf("state fields missing: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGetApplicationRetainsLatestCompletedOperation(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('a','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	op, err := CreateOperation(context.Background(), db, "a", "550e8400-e29b-41d4-a716-446655440098", "START")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetOperationState(context.Background(), db, op.ID, "SUCCEEDED", ""); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	(&Server{DB: db}).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/applications/a", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), op.ID) || strings.Contains(rr.Body.String(), `"reconciling":true`) {
		t.Fatalf("完了済みOperationを復元できません: status=%d body=%s", rr.Code, rr.Body.String())
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

func TestDerivedManagerUsesManifestPublication(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,manifest_service,manifest_port,created_at,updated_at) VALUES ('app-id','demo','https://github.com/a/b','main','frontend',4321,datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	manager := &DerivedManager{DB: db, GeneratedDir: t.TempDir(), BaseDomain: "example.internal", PublicAddress: "192.0.2.10"}
	if err := manager.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(manager.GeneratedDir); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != generatedDirMode {
		t.Fatalf("生成ディレクトリの権限が不正です: %o", got)
	}
	for _, name := range []string{"hosts", "Caddyfile"} {
		info, err := os.Stat(filepath.Join(manager.GeneratedDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != derivedFileMode {
			t.Fatalf("派生ファイルの権限が不正です: %s=%o", name, got)
		}
	}
	caddy, err := os.ReadFile(filepath.Join(manager.GeneratedDir, "Caddyfile"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(caddy); !strings.Contains(got, "reverse_proxy lws-app-id:4321") || strings.Contains(got, "lws-app-id:80") {
		t.Fatalf("manifest publication was not used: %s", got)
	}
}

func TestInfrastructureComposeUsesGeneratedCaddyfile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "infrastructure", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "lws-generated:/var/lib/lws/generated:ro") || strings.Contains(text, "./Caddyfile:/etc/caddy/Caddyfile:ro") {
		t.Fatal("Caddy is not configured to use the generated Caddyfile")
	}
}

func TestDerivedManagerValidatesAndReloadsInfrastructure(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,manifest_service,manifest_port,created_at,updated_at) VALUES ('app-id','demo','https://github.com/a/b','main','frontend',4321,datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{output: []byte(`[{"Id":"infra","Name":"infra","Config":{"Labels":{"com.labwebsystem.owner":"lws","com.labwebsystem.installation-id":"installation"}}}]`)}
	manager := &DerivedManager{
		DB: db, GeneratedDir: t.TempDir(), BaseDomain: "example.internal", PublicAddress: "192.0.2.10",
		Docker:         NewDockerResources(runner, "installation"),
		CaddyContainer: "caddy-container", CoreDNSContainer: "coredns-container",
	}
	if err := manager.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, call := range runner.calls {
		joined += strings.Join(call, " ") + "\n"
	}
	if !strings.Contains(joined, "exec caddy-container caddy validate --config /var/lib/lws/generated/Caddyfile --adapter caddyfile") ||
		!strings.Contains(joined, "exec caddy-container caddy reload --config /var/lib/lws/generated/Caddyfile --adapter caddyfile --address localhost:2019") ||
		!strings.Contains(joined, "kill --signal HUP coredns-container") {
		t.Fatalf("infrastructure reload was not performed: %s", joined)
	}
}

func TestDerivedManagerKeepsPreviousFilesWhenCaddyValidationFails(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,manifest_service,manifest_port,created_at,updated_at) VALUES ('app-id','demo','https://github.com/a/b','main','frontend',4321,datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts"), []byte("old hosts\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte("old caddy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &failingInfrastructureRunner{failOn: "caddy validate"}
	manager := &DerivedManager{DB: db, GeneratedDir: dir, BaseDomain: "example.internal", PublicAddress: "192.0.2.10", Docker: NewDockerResources(runner, "installation"), CaddyContainer: "caddy", CoreDNSContainer: "coredns"}
	if err := manager.Sync(context.Background()); err == nil {
		t.Fatal("Caddyfile検証失敗を成功扱いしました")
	}
	for name, want := range map[string]string{"hosts": "old hosts\n", "Caddyfile": "old caddy\n"} {
		got, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil || string(got) != want {
			t.Fatalf("%sが維持されていません: %q, %v", name, got, readErr)
		}
	}
}

func TestDerivedManagerKeepsPreviousFilesWhenCoreDNSReloadFails(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,manifest_service,manifest_port,created_at,updated_at) VALUES ('app-id','demo','https://github.com/a/b','main','frontend',4321,datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts"), []byte("old hosts\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte("old caddy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &failingInfrastructureRunner{failOn: "kill --signal HUP"}
	manager := &DerivedManager{DB: db, GeneratedDir: dir, BaseDomain: "example.internal", PublicAddress: "192.0.2.10", Docker: NewDockerResources(runner, "installation"), CaddyContainer: "caddy", CoreDNSContainer: "coredns"}
	if err := manager.Sync(context.Background()); err == nil {
		t.Fatal("CoreDNS再読込失敗を成功扱いしました")
	}
	for name, want := range map[string]string{"hosts": "old hosts\n", "Caddyfile": "old caddy\n"} {
		got, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil || string(got) != want {
			t.Fatalf("%sが維持されていません: %q, %v", name, got, readErr)
		}
	}
}

type recordingRunner struct {
	name   string
	args   []string
	output []byte
	calls  [][]string
}

type failingInfrastructureRunner struct {
	failOn string
}

func (r *failingInfrastructureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if strings.Contains(strings.Join(append([]string{name}, args...), " "), r.failOn) {
		return nil, fmt.Errorf("テスト用コマンド失敗")
	}
	return nil, nil
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
	_, err := (OSRunner{Timeout: 25 * time.Millisecond}).Run(context.Background(), "sleep", "1")
	if err == nil {
		t.Fatal("timeoutなしでコマンドが完了しました")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("timeoutが効いていません: %s", elapsed)
	}
}

func TestOSRunnerReturnsStandardErrorOutput(t *testing.T) {
	out, err := (OSRunner{}).Run(context.Background(), "sh", "-c", "printf 'network missing\\n' >&2; exit 1")
	if err == nil {
		t.Fatal("失敗するコマンドを成功扱いしました")
	}
	if !strings.Contains(string(out), "network missing") {
		t.Fatalf("標準エラーを取得できません: %q", out)
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

func TestGeneratedOverrideLabelsAllComposeServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.yaml")
	e := &RuntimeExecutor{InstallationID: "installation"}
	if err := e.GenerateOverrideWithServicesAndVolumes("app-id", "web", []string{"web", "worker"}, nil, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"worker"`) || strings.Count(text, "com.labwebsystem.app-id") != 2 {
		t.Fatalf("全serviceに所有labelがありません: %s", text)
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

func TestDockerNetworkCreatesWhenInspectReportsNotFound(t *testing.T) {
	r := &networkNotFoundRunner{}
	d := NewDockerResources(r, "installation")
	if err := d.EnsureNetwork(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 || !strings.Contains(strings.Join(r.calls[1], " "), "network create") {
		t.Fatalf("未存在networkを作成しませんでした: %v", r.calls)
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

func TestDockerTreatsExistingCaddyEndpointAsConnected(t *testing.T) {
	r := &errorRunner{output: []byte("Error response from daemon: endpoint with name caddy-container already exists in network lws-app-app-id-edge\n")}
	d := NewDockerResources(r, "installation")
	d.CaddyContainer = "caddy-container"
	if err := d.EnsureCaddyConnected(context.Background(), "app-id"); err != nil {
		t.Fatalf("接続済みCaddyを失敗扱いにしました: %v", err)
	}
}

func TestDockerRejectsForeignInfrastructureContainer(t *testing.T) {
	r := &recordingRunner{output: []byte(`[{"Id":"c1","Name":"foreign","Config":{"Labels":{"com.labwebsystem.owner":"other","com.labwebsystem.installation-id":"installation"}}}]`)}
	d := NewDockerResources(r, "installation")
	if err := d.VerifyInfrastructureContainer(context.Background(), "foreign"); err == nil {
		t.Fatal("LWS外のInfrastructure containerを受理しました")
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

func TestRuntimeReconcileActiveReconnectsRunningApplications(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,desired_state,observed_state,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main','RUNNING','RUNNING',datetime('now'),datetime('now'))`)
	if err != nil {
		t.Fatal(err)
	}
	r := &sequenceRunner{outputs: [][]byte{
		[]byte(`[{"Name":"lws-app-app-id-edge","Labels":{"com.labwebsystem.owner":"lws","com.labwebsystem.installation-id":"installation","com.labwebsystem.app-id":"app-id"}}]`),
		nil,
	}}
	e := &RuntimeExecutor{DB: db, Docker: NewDockerResources(r, "installation")}
	if err := e.ReconcileActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		joined = append(joined, strings.Join(call, " "))
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "network inspect lws-app-app-id-edge") || !strings.Contains(all, "network connect --alias lws-app-id lws-app-app-id-edge lws-caddy-1") {
		t.Fatalf("起動時のedge network再接続がありません: %s", all)
	}
}

func TestRuntimeReconcileActiveDoesNotConnectStoppedApplications(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,desired_state,observed_state,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main','STOPPED','STOPPED',datetime('now'),datetime('now'))`)
	if err != nil {
		t.Fatal(err)
	}
	r := &recordingRunner{}
	e := &RuntimeExecutor{DB: db, Docker: NewDockerResources(r, "installation")}
	if err := e.ReconcileActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("停止中アプリを再接続しました: %v", r.calls)
	}
}

func TestPurgeRequiresConfirmationAndRejectsActiveApplication(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, state := range []string{"ACTIVE", "UNREGISTERED"} {
		_, err = db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,registration_state,created_at,updated_at) VALUES (?,?,?,?,?,datetime('now'),datetime('now'))`, "app-"+state, "sub-"+strings.ToLower(state), "https://github.com/a/b", "main", state)
		if err != nil {
			t.Fatal(err)
		}
	}
	s := (&Server{DB: db}).Handler()
	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{path: "/api/v1/applications/app-UNREGISTERED:purge", body: `{"requestId":"550e8400-e29b-41d4-a716-446655440000"}`, want: 400},
		{path: "/api/v1/applications/app-ACTIVE:purge", body: `{"requestId":"550e8400-e29b-41d4-a716-446655440001","confirm":true}`, want: 202},
	} {
		r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("path=%s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestRuntimeCleansCaddyConnectionAfterComposeFailure(t *testing.T) {
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
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	composeRunner := &composeFailureRunner{}
	dockerRunner := &dockerBoundaryRunner{}
	e := &RuntimeExecutor{DB: db, Root: dir, Runner: composeRunner, Docker: NewDockerResources(dockerRunner, "installation")}
	err = e.reconcile(context.Background(), "app-id", source, runtime, "up", "-d")
	if err == nil {
		t.Fatal("Compose failureを成功扱いしました")
	}
	joined := strings.Join(dockerRunner.calls, "\n")
	if !strings.Contains(joined, "network disconnect -f lws-app-app-id-edge lws-caddy-1") {
		t.Fatalf("Compose失敗後にCaddy接続を解除していません: %s", joined)
	}
}

func TestRuntimeUnregisterKeepsCaddyConnectedWhenComposeDownFails(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "app-id")
	source := filepath.Join(root, "source")
	runtime := filepath.Join(root, "runtime")
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
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,desired_state,registration_state,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main','RUNNING','ACTIVE',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	docker := &dockerBoundaryRunner{}
	e := &RuntimeExecutor{DB: db, Root: dir, Runner: &composeFailureRunner{}, Docker: NewDockerResources(docker, "installation")}
	err = e.Run(context.Background(), Operation{ApplicationID: "app-id", Kind: "UNREGISTER"})
	if err == nil {
		t.Fatal("Compose停止失敗を成功扱いしました")
	}
	for _, call := range docker.calls {
		if strings.Contains(call, "network disconnect") {
			t.Fatalf("Compose停止前にCaddyを切断しました: %s", call)
		}
	}
}

func TestRuntimeUnregisterPreservesSourceAndRegisterRestoresIt(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "app-id")
	source := filepath.Join(root, "source")
	runtime := filepath.Join(root, "runtime")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "compose.yaml"), []byte("services:\n  web:\n    image: example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,registration_state,created_at,updated_at) VALUES ('app-id','app','https://github.com/a/b','main','ACTIVE',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	e := &RuntimeExecutor{DB: db, Root: dir, Runner: &composeSuccessRunner{}}
	if err := e.Run(context.Background(), Operation{ApplicationID: "app-id", Kind: "UNREGISTER"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(source, "compose.yaml")); err != nil {
		t.Fatalf("登録解除でsourceを削除しました: %v", err)
	}
	var state string
	if err := db.QueryRow(`SELECT registration_state FROM applications WHERE id='app-id'`).Scan(&state); err != nil || state != "UNREGISTERED" {
		t.Fatalf("登録解除状態=%q err=%v", state, err)
	}
	if err := e.Run(context.Background(), Operation{ApplicationID: "app-id", Kind: "REGISTER"}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT registration_state FROM applications WHERE id='app-id'`).Scan(&state); err != nil || state != "ACTIVE" {
		t.Fatalf("再登録状態=%q err=%v", state, err)
	}
}

type errorRunner struct {
	output []byte
	calls  [][]string
}

type networkNotFoundRunner struct{ calls [][]string }

func (r *networkNotFoundRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(r.calls) == 1 {
		return []byte("Error response from daemon: network lws-app-app-edge not found\\n"), fmt.Errorf("exit status 1")
	}
	return nil, nil
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

type composeFailureRunner struct{}

type composeSuccessRunner struct{}

func (r *composeSuccessRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (r *composeFailureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "docker" && len(args) > 0 && args[0] == "compose" {
		if strings.Contains(strings.Join(args, " "), "config") {
			return []byte(`{"services":{"web":{"image":"example","networks":{"lws-edge":{}}}}}`), nil
		}
		return nil, fmt.Errorf("compose failure")
	}
	return nil, nil
}

type dockerBoundaryRunner struct{ calls []string }

func (r *dockerBoundaryRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, call)
	switch {
	case strings.Contains(call, "docker ps -a"):
		return nil, nil
	case strings.Contains(call, "docker network inspect"):
		return []byte(`[{"Name":"lws-app-app-id-edge","Labels":{"com.labwebsystem.owner":"lws","com.labwebsystem.installation-id":"installation","com.labwebsystem.app-id":"app-id"}}]`), nil
	default:
		return nil, nil
	}
}

func (r *sequenceRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	output := r.outputs[0]
	r.outputs = r.outputs[1:]
	return output, nil
}
