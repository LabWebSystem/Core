package backend

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newLogStoreForTest(t *testing.T) (*sql.DB, *LogStore) {
	t.Helper()
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES ('app','app','https://github.com/a/b','main',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	return db, NewLogStore(db)
}

func TestLogStoreOrdersFiltersAndCascades(t *testing.T) {
	db, store := newLogStoreForTest(t)
	defer db.Close()
	base := time.Date(2026, 8, 23, 1, 2, 3, 0, time.UTC)
	for _, entry := range []StoredLogEntry{
		{OccurredAt: base, Component: "application", Level: "info", ApplicationID: "app", Service: "web", Message: "web"},
		{OccurredAt: base.Add(time.Second), Component: "application", Level: "info", ApplicationID: "app", Service: "worker", Message: "worker"},
		{OccurredAt: base.Add(2 * time.Second), Component: "backend", Level: "info", ApplicationID: "app", Message: "operation"},
	} {
		if _, err := store.Append(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.Query(context.Background(), LogQuery{ApplicationID: "app", View: "application", Limit: 1})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Message != "web" || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	next, err := store.Query(context.Background(), LogQuery{ApplicationID: "app", View: "application", Cursor: page.NextCursor})
	if err != nil || len(next.Entries) != 1 || next.Entries[0].Message != "worker" {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	filtered, err := store.Query(context.Background(), LogQuery{ApplicationID: "app", View: "application", Service: "worker", StartAt: ptrTime(base)})
	if err != nil || len(filtered.Entries) != 1 || filtered.Entries[0].Message != "worker" {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
	if _, err := store.Query(context.Background(), LogQuery{ApplicationID: "app", View: "application", Service: "Worker"}); err == nil {
		t.Fatal("不正serviceを受理しました")
	}
	if _, err := db.Exec(`DELETE FROM applications WHERE id='app'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM log_entries`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("ログがcascade削除されません: count=%d err=%v", count, err)
	}
}

func TestChunkRedactorNeverReturnsSplitSecret(t *testing.T) {
	r := NewChunkRedactor([]string{"token-value"})
	got := r.Feed("before token-") + r.Feed("value after") + r.Flush()
	if strings.Contains(got, "token-value") || got != "before [REDACTED] after" {
		t.Fatalf("secretを安全にマスクできません: %q", got)
	}
}

func TestLogStoreMasksConfiguredSecretBeforeDatabaseWrite(t *testing.T) {
	db, store := newLogStoreForTest(t)
	defer db.Close()
	key := []byte("0123456789abcdef0123456789abcdef")
	encrypted, err := Encrypt(key, []byte("top-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO application_variables(application_id,name,value,is_secret,updated_at) VALUES('app','TOKEN',?,1,datetime('now'))`, encrypted); err != nil {
		t.Fatal(err)
	}
	store.SecretKey = key
	entry, err := store.Append(context.Background(), StoredLogEntry{Component: "application", Level: "info", ApplicationID: "app", Service: "web", Message: "TOKEN=top-secret"})
	if err != nil || strings.Contains(entry.Message, "top-secret") {
		t.Fatalf("保存前にsecretを除去できません: entry=%+v err=%v", entry, err)
	}
	var stored string
	if err := db.QueryRow(`SELECT message FROM log_entries WHERE id=?`, entry.ID).Scan(&stored); err != nil || strings.Contains(stored, "top-secret") {
		t.Fatalf("DBにsecretが残りました: %q err=%v", stored, err)
	}
}

func TestLogEntriesAPIAndSSEResumeFromCursor(t *testing.T) {
	db, store := newLogStoreForTest(t)
	defer db.Close()
	server := NewServer(db, nil)
	server.Logs = store
	server.worker.logs = store
	first, err := store.Append(context.Background(), StoredLogEntry{Component: "application", Level: "info", ApplicationID: "app", Service: "web", Message: "first"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/applications/app/logEntries?view=application", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "first") || !strings.Contains(response.Body.String(), "liveCursor") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	watchRequest := httptest.NewRequest(http.MethodGet, "/api/v1/applications/app/logEntries:watch?view=application&after="+first.Cursor, nil)
	ctx, cancel := context.WithCancel(watchRequest.Context())
	defer cancel()
	watchRequest = watchRequest.WithContext(ctx)
	watch := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { server.Handler().ServeHTTP(watch, watchRequest); close(done) }()
	time.Sleep(20 * time.Millisecond)
	if _, err := store.Append(context.Background(), StoredLogEntry{Component: "application", Level: "info", ApplicationID: "app", Service: "web", Message: "second"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(watch.Body.String(), "second") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if strings.Contains(watch.Body.String(), "first") || !strings.Contains(watch.Body.String(), "second") {
		t.Fatalf("SSE cursor再開が不正です: %s", watch.Body.String())
	}
}

func TestRuntimeMonitorCollectsOnlyOwnedApplicationContainer(t *testing.T) {
	db, store := newLogStoreForTest(t)
	defer db.Close()
	source := &fakeDockerRuntimeSource{containers: []MonitoredContainer{
		{ID: "owned", Name: "web", TTY: true, Labels: map[string]string{ownerLabel: "lws", installationIDLabel: "installation", applicationIDLabel: "app", composeServiceLabel: "web"}},
		{ID: "outside", Name: "outside", TTY: true, Labels: map[string]string{ownerLabel: "other", installationIDLabel: "installation"}},
	}}
	monitor := NewRuntimeMonitor(source, store, db, "installation", "example.internal", nil)
	monitor.RetryDelay = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { monitor.Run(ctx); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		page, err := store.Query(context.Background(), LogQuery{ApplicationID: "app", View: "application"})
		if err == nil && len(page.Entries) == 1 {
			if page.Entries[0].Service != "web" || page.Entries[0].Message != "owned log" {
				t.Fatalf("関連付けが不正です: %+v", page.Entries[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("所有containerログを収集できません: page=%+v err=%v", page, err)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if source.logCalls("outside") != 0 {
		t.Fatal("LWS外containerを監視しました")
	}
}

type fakeDockerRuntimeSource struct {
	containers []MonitoredContainer
	calls      map[string]int
}

func (s *fakeDockerRuntimeSource) ListContainers(context.Context) ([]MonitoredContainer, error) {
	return s.containers, nil
}
func (s *fakeDockerRuntimeSource) Events(context.Context) (<-chan DockerRuntimeEvent, <-chan error) {
	events := make(chan DockerRuntimeEvent)
	errors := make(chan error)
	close(events)
	close(errors)
	return events, errors
}
func (s *fakeDockerRuntimeSource) ContainerLogs(_ context.Context, id string) (io.ReadCloser, error) {
	if s.calls == nil {
		s.calls = map[string]int{}
	}
	s.calls[id]++
	return io.NopCloser(strings.NewReader(id + " log\n")), nil
}
func (s *fakeDockerRuntimeSource) logCalls(id string) int { return s.calls[id] }

func ptrTime(value time.Time) *time.Time { return &value }
