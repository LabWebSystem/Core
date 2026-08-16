package backend

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBInitializesWithWALAndForeignKeys(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fk, wal, err := sqlitePragmas(context.Background(), db)
	if err != nil || !fk || !wal {
		t.Fatalf("pragmas fk=%v wal=%v err=%v", fk, wal, err)
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
func TestValidateManifest(t *testing.T) {
	m, err := ValidateManifest([]byte("apiVersion: lws/v1\nmetadata:\n  name: Demo\n  description: test\npublic:\n  service: web\n  port: 3000\n"))
	if err != nil || m.Public.Port != 3000 {
		t.Fatal(m, err)
	}
	_, err = ValidateManifest([]byte("apiVersion: wrong\nmetadata: {}\npublic: {}\n"))
	if err == nil {
		t.Fatal("invalid manifest accepted")
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
func TestHealth(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/health/live", nil))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/health/ready", nil))
	if rr.Code != 503 {
		t.Fatal(rr.Code)
	}
}
func TestHealthRejectsOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "/health/live", nil)
	r.Header.Set("Origin", "https://bad")
	rr := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(rr, r)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "ORIGIN_FORBIDDEN") {
		t.Fatal(rr.Code, rr.Body.String())
	}
}
func TestManifestSymlinkIsNotUsed(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "manifest")
	if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestComposeRejectsExternalFeatures(t *testing.T) {
	err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    env_file: .env\n"))
	if err == nil {
		t.Fatal("env_file accepted")
	}
	if err := ValidateComposeSource("/tmp/project", []byte("services:\n  app:\n    build:\n      context: ../../outside\n")); err == nil {
		t.Fatal("outside context accepted")
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
	name string
	args []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = args
	return nil, nil
}
func TestRuntimeUsesOwnedComposeArguments(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	runtime := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "compose.yaml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &recordingRunner{}
	e := &RuntimeExecutor{Root: dir, Runner: r}
	if err := e.reconcile(context.Background(), "app-id", source, runtime, "up", "-d"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(r.args, " ")
	if !strings.Contains(got, "--project-name lws-app-app-id") || !strings.Contains(got, "-f "+filepath.Join(source, "compose.yaml")) || strings.Contains(got, "--volumes") {
		t.Fatal(got)
	}
}
