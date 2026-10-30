package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestInspectImport(t *testing.T) {
	dir := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("compose.yaml", "services:\n  web:\n    image: nginx\n    expose: [8080]\n  worker:\n    image: example\n")
	info, err := InspectImport(dir)
	if err != nil || info.Manifest != nil || len(info.ComposeFiles) != 1 || len(info.ComposeFiles[0].Services) != 2 {
		t.Fatalf("manifestなしの検出結果が不正です: %+v %v", info, err)
	}
	write("lws.manifest.yaml", "apiVersion: lws/v1\ncomposeFile: compose.yaml\nmetadata:\n  name: App\n  description: test\npublic:\n  service: web\n  port: 8080\n")
	info, err = InspectImport(dir)
	if err != nil || info.Manifest == nil || info.Manifest.ComposeFile != "compose.yaml" || info.Manifest.Public.Port != 8080 {
		t.Fatalf("manifestの検出結果が不正です: %+v %v", info, err)
	}
	write("lws.manifest.yaml", "broken: true\n")
	if _, err := InspectImport(dir); err == nil {
		t.Fatal("不正なmanifestを受理しました")
	}
}

func TestInspectImportRequiresCompose(t *testing.T) {
	if _, err := InspectImport(t.TempDir()); err == nil {
		t.Fatal("Composeのないsourceを受理しました")
	}
}

func TestImportStageExpirationAndLimit(t *testing.T) {
	server := &Server{AppsRoot: t.TempDir()}
	var first string
	for i := 0; i < 20; i++ {
		_, stage, err := server.newImportStage()
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = stage
		}
	}
	if _, _, err := server.newImportStage(); err == nil {
		t.Fatal("未完了の取り込み数の上限を超過しました")
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(first, old, old); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.newImportStage(); err != nil {
		t.Fatalf("期限切れの取り込みを整理できません: %v", err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("期限切れの取り込みが残っています")
	}
}

func TestDiscardImport(t *testing.T) {
	server := &Server{AppsRoot: t.TempDir()}
	id, stage, err := server.newImportStage()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/imports/"+id, nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("取り込みを破棄できません: %d %s", response.Code, response.Body)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("取り込み領域が残っています")
	}
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("再破棄に失敗しました: %d", response.Code)
	}
}

func TestValidateImportedSelection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selection := ImportSelection{ComposeFile: "compose.yaml", DisplayName: "デモ", PublicService: "web", PublicPort: 8080}
	if err := ValidateImportedSelection(dir, selection); err != nil {
		t.Fatalf("有効な選択を拒否しました: %v", err)
	}
	bad := selection
	bad.PublicService = "missing"
	if err := ValidateImportedSelection(dir, bad); err == nil {
		t.Fatal("存在しないサービスを受理しました")
	}
	bad = selection
	bad.OverrideFiles = []string{"../escape.yml"}
	if err := ValidateImportedSelection(dir, bad); err == nil {
		t.Fatal("不正なoverrideを受理しました")
	}
	if err := os.WriteFile(filepath.Join(dir, "lws.manifest.yaml"), []byte("invalid: true"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImportedSelection(dir, selection); err == nil {
		t.Fatal("不正なmanifestを受理しました")
	}
}

func TestPrepareImports(t *testing.T) {
	server := &Server{AppsRoot: t.TempDir(), SourceRunner: &cloneValidationRunner{}}
	zipData := testArchive(t, archiveEntry{name: "app/compose.yaml", body: "services:\n  web:\n    image: nginx\n"})
	for _, tc := range []struct {
		name, contentType string
		body              []byte
		method            http.HandlerFunc
	}{
		{"zip", "application/zip", zipData, server.prepareZipImport},
		{"github", "application/json", []byte(`{"repositoryUrl":"https://github.com/a/b","ref":"main"}`), server.prepareGitImport},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			response := httptest.NewRecorder()
			tc.method(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("取り込み準備に失敗しました: %d %s", response.Code, response.Body)
			}
			var result struct {
				ID string `json:"importId"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.ID == "" {
				t.Fatalf("importIdを読み取れません: %s", response.Body)
			}
			if _, err := os.Stat(filepath.Join(server.AppsRoot, ".imports", result.ID, "source", "compose.yaml")); err != nil {
				t.Fatal(err)
			}
		})
	}
	bad := testArchive(t, archiveEntry{name: "../escape"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bad))
	req.Header.Set("Content-Type", "application/zip")
	response := httptest.NewRecorder()
	server.prepareZipImport(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "ZIP") {
		t.Fatalf("危険なZIPを拒否できません: %d %s", response.Code, response.Body)
	}
}

func TestImportHTTPRoutes(t *testing.T) {
	server := &Server{AppsRoot: t.TempDir(), SourceRunner: &cloneValidationRunner{}}
	data := testArchive(t, archiveEntry{name: "compose.yaml", body: "services:\n  web:\n    image: nginx\n"})
	for _, tc := range []struct {
		path, contentType string
		body              []byte
	}{
		{"/api/v1/imports/zip", "application/zip", data},
		{"/api/v1/imports/github", "application/json", []byte(`{"repositoryUrl":"https://github.com/a/b","ref":"main"}`)},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.contentType)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.path, response.Code, response.Body)
		}
	}
}

func TestImportCreateWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner := &featureRunner{}
	runtime := &RuntimeExecutor{DB: db, Root: filepath.Join(dir, "apps"), Runner: runner}
	server := NewServer(db, runtime.Run)
	server.AppsRoot = runtime.Root
	data := testArchive(t, archiveEntry{name: "compose.yaml", body: "services:\n  web:\n    image: nginx\n"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/imports/zip", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/zip")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("取り込み: %d %s", response.Code, response.Body)
	}
	var prepared struct {
		ID string `json:"importId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	body := `{"importId":"` + prepared.ID + `","subdomain":"sample","composeFile":"compose.yaml","displayName":"サンプル","description":"","publicService":"web","publicPort":8080,"requestId":"550e8400-e29b-41d4-a716-446655440220"}`
	request = httptest.NewRequest(http.MethodPost, "/api/v1/applications", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("登録: %d %s", response.Code, response.Body)
	}
	var op struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &op); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var state, message string
		if err := db.QueryRow(`SELECT state,error_message FROM operations WHERE id=?`, strings.TrimPrefix(op.Name, "operations/")).Scan(&state, &message); err == nil && (state == "SUCCEEDED" || state == "FAILED") {
			if state != "SUCCEEDED" {
				t.Fatalf("登録失敗: %s", message)
			}
			var id, kind, service, name, state string
			var port int
			if err := db.QueryRow(`SELECT id,source_kind,public_service,public_port,manifest_name,registration_state FROM applications WHERE subdomain='sample'`).Scan(&id, &kind, &service, &port, &name, &state); err != nil {
				t.Fatal(err)
			}
			if kind != "zip" || service != "web" || port != 8080 || name != "サンプル" || state != "CONFIGURING" {
				t.Fatalf("登録状態が不正: %s %s %d %s %s", kind, service, port, name, state)
			}
			if _, err := os.Stat(filepath.Join(runtime.Root, id, "source", "compose.yaml")); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("登録Operationが完了しません")
}

func TestZipImportUpdate(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runtime := &RuntimeExecutor{DB: db, Root: filepath.Join(dir, "apps"), Runner: &featureRunner{}}
	id := "550e8400-e29b-41d4-a716-446655440221"
	source := filepath.Join(runtime.Root, id, "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "compose.yaml"), []byte("services:\n  old:\n    image: nginx\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,source_kind,compose_file,manifest_name,manifest_service,manifest_port,public_service,public_port,registration_state,desired_state,created_at,updated_at) VALUES(?, 'sample','','','zip','compose.yaml','old','old',8000,'old',8000,'ACTIVE','STOPPED',datetime('now'),datetime('now'))`, id); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, runtime.Run)
	server.AppsRoot = runtime.Root
	data := testArchive(t, archiveEntry{name: "compose.yaml", body: "services:\n  web:\n    image: nginx\n"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/zip", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/zip")
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	var prepared struct {
		ID string `json:"importId"`
	}
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &prepared) != nil {
		t.Fatalf("準備: %d %s", res.Code, res.Body)
	}
	body := `{"importId":"` + prepared.ID + `","composeFile":"compose.yaml","displayName":"new","publicService":"web","publicPort":8080,"requestId":"550e8400-e29b-41d4-a716-446655440222"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/applications/"+id+"/imports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != 202 {
		t.Fatalf("更新受付: %d %s", res.Code, res.Body)
	}
	var op struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &op); err != nil {
		t.Fatal(err)
	}
	if result := waitFeatureOperation(t, db, op.Name); !strings.HasPrefix(result, "SUCCEEDED\x00") {
		t.Fatalf("更新失敗: %s", result)
	}
	var name, service, desired string
	var port int
	if err := db.QueryRow(`SELECT manifest_name,public_service,public_port,desired_state FROM applications WHERE id=?`, id).Scan(&name, &service, &port, &desired); err != nil {
		t.Fatal(err)
	}
	if name != "new" || service != "web" || port != 8080 || desired != "STOPPED" {
		t.Fatalf("更新状態が不正: %s %s %d %s", name, service, port, desired)
	}
	contents, err := os.ReadFile(filepath.Join(source, "compose.yaml"))
	if err != nil || !strings.Contains(string(contents), "web:") {
		t.Fatalf("sourceが更新されません: %s %v", contents, err)
	}
}

func TestZipUpdateRestoresSourceAndRuntimeOnFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	appID := "550e8400-e29b-41d4-a716-446655440223"
	opID := "550e8400-e29b-41d4-a716-446655440224"
	appRoot := filepath.Join(dir, "apps", appID)
	for _, folder := range []string{"source", "runtime", "pending-" + opID + "/source"} {
		if err := os.MkdirAll(filepath.Join(appRoot, folder), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(appRoot, "source", "compose.yaml"), []byte("services:\n  old:\n    image: nginx\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "runtime", "lws.override.yaml"), []byte("old override"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "pending-"+opID, "source", "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,source_kind,compose_file,manifest_name,manifest_service,manifest_port,public_service,public_port,desired_state,observed_state,created_at,updated_at) VALUES(?, 'sample','','','zip','compose.yaml','old','old',8000,'old',8000,'RUNNING','RUNNING',datetime('now'),datetime('now'))`, appID); err != nil {
		t.Fatal(err)
	}
	selection := ImportSelection{ComposeFile: "compose.yaml", DisplayName: "new", PublicService: "web", PublicPort: 8080}
	payload, _ := json.Marshal(struct {
		Prepared  bool            `json:"prepared"`
		Selection ImportSelection `json:"selection"`
	}{true, selection})
	runtime := &RuntimeExecutor{DB: db, Root: filepath.Dir(appRoot), Runner: &composeFailureRunner{}}
	if err := runtime.Run(context.Background(), Operation{ID: opID, ApplicationID: appID, Kind: "SYNC", Payload: string(payload)}); err == nil {
		t.Fatal("失敗を検出できません")
	}
	old, err := os.ReadFile(filepath.Join(appRoot, "source", "compose.yaml"))
	if err != nil || !strings.Contains(string(old), "old:") {
		t.Fatalf("sourceが復元されません: %s %v", old, err)
	}
	old, err = os.ReadFile(filepath.Join(appRoot, "runtime", "lws.override.yaml"))
	if err != nil || string(old) != "old override" {
		t.Fatalf("実行設定が復元されません: %s %v", old, err)
	}
	var name, service, desired, observed string
	var port int
	if err := db.QueryRow(`SELECT manifest_name,public_service,public_port,desired_state,observed_state FROM applications WHERE id=?`, appID).Scan(&name, &service, &port, &desired, &observed); err != nil {
		t.Fatal(err)
	}
	if name != "old" || service != "old" || port != 8000 || desired != "RUNNING" || observed != "RUNNING" {
		t.Fatalf("状態が復元されません: %s %s %d %s %s", name, service, port, desired, observed)
	}
}

type importTestRunner struct {
	featureRunner
	manifest bool
}

func (r *importTestRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name != "git" {
		return r.featureRunner.Run(ctx, name, args...)
	}
	dest := args[len(args)-1]
	if err := os.MkdirAll(dest, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dest, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0600); err != nil {
		return nil, err
	}
	if r.manifest {
		if err := os.WriteFile(filepath.Join(dest, "lws.manifest.yaml"), []byte(importTestManifest), 0600); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

const importTestManifest = "apiVersion: lws/v1\ncomposeFile: compose.yaml\nmetadata:\n  name: App\n  description: test\npublic:\n  service: web\n  port: 8080\n"

func TestImportVariantsRegisterAndStart(t *testing.T) {
	for _, kind := range []string{"github", "zip"} {
		for _, hasManifest := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_manifest_%t", kind, hasManifest), func(t *testing.T) {
				dir := t.TempDir()
				db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				runner := &importTestRunner{manifest: hasManifest}
				runtime := &RuntimeExecutor{DB: db, Root: filepath.Join(dir, "apps"), Runner: runner}
				server := NewServer(db, runtime.Run)
				server.AppsRoot, server.SourceRunner = runtime.Root, runner
				var request *http.Request
				if kind == "github" {
					request = httptest.NewRequest(http.MethodPost, "/api/v1/imports/github", strings.NewReader(`{"repositoryUrl":"https://github.com/a/b","ref":"main"}`))
					request.Header.Set("Content-Type", "application/json")
				} else {
					entries := []archiveEntry{{name: "compose.yaml", body: "services:\n  web:\n    image: nginx\n"}}
					if hasManifest {
						entries = append(entries, archiveEntry{name: "lws.manifest.yaml", body: importTestManifest})
					}
					request = httptest.NewRequest(http.MethodPost, "/api/v1/imports/zip", bytes.NewReader(testArchive(t, entries...)))
					request.Header.Set("Content-Type", "application/zip")
				}
				response := httptest.NewRecorder()
				server.Handler().ServeHTTP(response, request)
				var prepared struct {
					ID       string    `json:"importId"`
					Manifest *Manifest `json:"manifest"`
				}
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &prepared) != nil || (prepared.Manifest != nil) != hasManifest {
					t.Fatalf("検査: %d %s", response.Code, response.Body)
				}
				body, _ := json.Marshal(map[string]any{"importId": prepared.ID, "subdomain": "sample", "composeFile": "compose.yaml", "displayName": "App", "publicService": "web", "publicPort": 8080, "requestId": uuid.NewString()})
				request = httptest.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response = httptest.NewRecorder()
				server.Handler().ServeHTTP(response, request)
				var op struct {
					Name string `json:"name"`
				}
				if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &op) != nil {
					t.Fatalf("登録: %d %s", response.Code, response.Body)
				}
				if result := waitFeatureOperation(t, db, op.Name); !strings.HasPrefix(result, "SUCCEEDED\x00") {
					t.Fatalf("登録: %s", result)
				}
				var id string
				if err := db.QueryRow(`SELECT id FROM applications WHERE subdomain='sample'`).Scan(&id); err != nil {
					t.Fatal(err)
				}
				request = httptest.NewRequest(http.MethodPost, "/api/v1/applications/"+id+":start", strings.NewReader(`{"requestId":"`+uuid.NewString()+`"}`))
				request.Header.Set("Content-Type", "application/json")
				response = httptest.NewRecorder()
				server.Handler().ServeHTTP(response, request)
				if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &op) != nil {
					t.Fatalf("起動: %d %s", response.Code, response.Body)
				}
				if result := waitFeatureOperation(t, db, op.Name); !strings.HasPrefix(result, "SUCCEEDED\x00") {
					t.Fatalf("起動: %s", result)
				}
				var desired string
				if err := db.QueryRow(`SELECT desired_state FROM applications WHERE id=?`, id).Scan(&desired); err != nil || desired != "RUNNING" {
					t.Fatalf("起動状態: %s %v", desired, err)
				}
				if kind == "github" {
					request = httptest.NewRequest(http.MethodPost, "/api/v1/applications/"+id+":sync", strings.NewReader(`{"requestId":"`+uuid.NewString()+`"}`))
					request.Header.Set("Content-Type", "application/json")
					response = httptest.NewRecorder()
					server.Handler().ServeHTTP(response, request)
					if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &op) != nil {
						t.Fatalf("同期: %d %s", response.Code, response.Body)
					}
					if result := waitFeatureOperation(t, db, op.Name); !strings.HasPrefix(result, "SUCCEEDED\x00") {
						t.Fatalf("同期: %s", result)
					}
				}
			})
		}
	}
}

func TestWorkerKeepsObservedStateAfterRestoredZipUpdate(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,observed_state,created_at,updated_at) VALUES('app','sample','','','RUNNING',datetime('now'),datetime('now'))`)
	if err != nil {
		t.Fatal(err)
	}
	op, err := CreateOperationWithPayload(context.Background(), db, "app", uuid.NewString(), "SYNC", "test", `{"prepared":true}`)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, func(context.Context, Operation) error { return fmt.Errorf("更新失敗") })
	worker.execute(op)
	var observed, message string
	if err := db.QueryRow(`SELECT observed_state,latest_error FROM applications WHERE id='app'`).Scan(&observed, &message); err != nil || observed != "RUNNING" || message != "更新失敗" {
		t.Fatalf("実行状態が維持されません: %s %s %v", observed, message, err)
	}
}

type failFirstComposeUpRunner struct{ upCalls int }

func (r *failFirstComposeUpRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "docker" && strings.Contains(strings.Join(args, " "), "config") {
		return []byte(`{"services":{"web":{"image":"nginx"},"old":{"image":"nginx"}}}`), nil
	}
	if name == "docker" && strings.Contains(strings.Join(args, " "), " up -d") {
		r.upCalls++
		if r.upCalls == 1 {
			return nil, fmt.Errorf("更新先の起動失敗")
		}
	}
	return nil, nil
}

func TestRunningZipUpdateRestoresPreviousContainers(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	appID, opID := uuid.NewString(), uuid.NewString()
	appRoot := filepath.Join(dir, "apps", appID)
	for _, folder := range []string{"source", "runtime", "pending-" + opID + "/source"} {
		if err := os.MkdirAll(filepath.Join(appRoot, folder), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{
		"source/compose.yaml":                      "services:\n  old:\n    image: nginx\n",
		"runtime/lws.override.yaml":                "services: {}\n",
		"pending-" + opID + "/source/compose.yaml": "services:\n  web:\n    image: nginx\n",
	} {
		if err := os.WriteFile(filepath.Join(appRoot, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO applications(id,subdomain,repository_url,git_ref,source_kind,compose_file,manifest_name,manifest_service,manifest_port,public_service,public_port,desired_state,observed_state,created_at,updated_at) VALUES(?, 'sample','','','zip','compose.yaml','old','old',8000,'old',8000,'RUNNING','RUNNING',datetime('now'),datetime('now'))`, appID); err != nil {
		t.Fatal(err)
	}
	selection := ImportSelection{ComposeFile: "compose.yaml", DisplayName: "new", PublicService: "web", PublicPort: 8080}
	payload, _ := json.Marshal(struct {
		Prepared  bool            `json:"prepared"`
		Selection ImportSelection `json:"selection"`
	}{true, selection})
	runner := &failFirstComposeUpRunner{}
	runtime := &RuntimeExecutor{DB: db, Root: filepath.Dir(appRoot), Runner: runner}
	err = runtime.Run(context.Background(), Operation{ID: opID, ApplicationID: appID, Kind: "SYNC", Payload: string(payload)})
	if err == nil || runner.upCalls != 2 {
		t.Fatalf("更新失敗後に旧containerを復元できません: %v calls=%d", err, runner.upCalls)
	}
	var name, service string
	var port int
	if err := db.QueryRow(`SELECT manifest_name,public_service,public_port FROM applications WHERE id=?`, appID).Scan(&name, &service, &port); err != nil || name != "old" || service != "old" || port != 8000 {
		t.Fatalf("旧設定を復元できません: %s %s %d %v", name, service, port, err)
	}
	contents, err := os.ReadFile(filepath.Join(appRoot, "source", "compose.yaml"))
	if err != nil || !strings.Contains(string(contents), "old:") {
		t.Fatalf("旧sourceを復元できません: %s %v", contents, err)
	}
}
