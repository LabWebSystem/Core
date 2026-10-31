package backend

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type ImportComposeFile struct {
	Name       string         `json:"name"`
	Services   []string       `json:"services"`
	Interfaces []WebInterface `json:"interfaces"`
}

type SourceInspection struct {
	Manifest     *Manifest           `json:"manifest"`
	ComposeFiles []ImportComposeFile `json:"composeFiles"`
}

type importMetadata struct {
	Kind          string `json:"kind"`
	RepositoryURL string `json:"repositoryUrl,omitempty"`
	Ref           string `json:"ref,omitempty"`
}

type ImportSelection struct {
	ComposeFile   string   `json:"composeFile"`
	OverrideFiles []string `json:"overrideFiles"`
	DisplayName   string   `json:"displayName"`
	Description   string   `json:"description"`
	PublicService string   `json:"publicService"`
	PublicPort    int      `json:"publicPort"`
}

type updateImportInput struct {
	ImportID string `json:"importId"`
	ImportSelection
	RequestID string `json:"requestId"`
}

// ValidateImportedSelectionは未信頼sourceと利用者が選択した公開先を検証する。
func ValidateImportedSelection(root string, selection ImportSelection) error {
	if selection.DisplayName == "" || strings.TrimSpace(selection.DisplayName) != selection.DisplayName || len([]rune(selection.DisplayName)) > 80 || len([]rune(selection.Description)) > 500 || strings.ContainsAny(selection.Description, "\r\n\x00") || !servicePattern.MatchString(selection.PublicService) || selection.PublicPort < 1 || selection.PublicPort > 65535 {
		return NewValidationError("selection", "アプリ名、説明または公開先が不正です", "INVALID_ARGUMENT")
	}
	if !containsComposeFile(selection.ComposeFile) {
		return NewValidationError("composeFile", "Composeファイル名が不正です", "INVALID_ARGUMENT")
	}
	seen := map[string]bool{selection.ComposeFile: true}
	for _, file := range selection.OverrideFiles {
		if !containsComposeFile(file) || seen[file] {
			return NewValidationError("overrideFiles", "override Composeファイルが不正です", "INVALID_ARGUMENT")
		}
		seen[file] = true
	}
	if _, _, err := validatePreparedSource(root, selection.ComposeFile, selection.OverrideFiles, true); err != nil {
		return err
	}
	sources, err := ReadComposeSources(root, selection.ComposeFile, selection.OverrideFiles)
	if err != nil {
		return err
	}
	if !ComposeHasService(sources, selection.PublicService) {
		return NewValidationError("publicService", "公開するサービスがComposeに存在しません", "INVALID_ARGUMENT")
	}
	return nil
}

func (s *Server) prepareZipImport(w http.ResponseWriter, r *http.Request) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/zip" {
		writeAPIError(w, 400, "INVALID_CONTENT_TYPE", "ZIPファイルを指定してください", "contentType")
		return
	}
	if r.ContentLength > MaxArchiveBytes {
		writeAPIError(w, 400, "INVALID_ARCHIVE", "ZIPのサイズが上限を超えています", "file")
		return
	}
	id, stage, err := s.newImportStage()
	if err != nil {
		writeAPIError(w, 500, "IMPORT_FAILED", "取り込み領域を作成できません", "")
		return
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(stage)
		}
	}()
	archivePath := filepath.Join(stage, "upload.zip")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		writeAPIError(w, 500, "IMPORT_FAILED", "ZIPを保存できません", "")
		return
	}
	size, copyErr := io.Copy(file, io.LimitReader(r.Body, MaxArchiveBytes+1))
	if copyErr != nil || size > MaxArchiveBytes {
		_ = file.Close()
		writeAPIError(w, 400, "INVALID_ARCHIVE", "ZIPの読み込みに失敗したかサイズが上限を超えています", "file")
		return
	}
	work := filepath.Join(stage, "extract")
	if err := os.Mkdir(work, 0700); err != nil {
		_ = file.Close()
		writeAPIError(w, 500, "IMPORT_FAILED", "ZIPの展開先を作成できません", "")
		return
	}
	root, extractErr := ExtractArchive(file, size, work)
	closeErr := file.Close()
	_ = os.Remove(archivePath)
	if extractErr != nil {
		writeValidationError(w, extractErr)
		return
	}
	if closeErr != nil || os.Rename(root, filepath.Join(stage, "source")) != nil {
		writeAPIError(w, 500, "IMPORT_FAILED", "ZIPの内容を保存できません", "")
		return
	}
	_ = os.RemoveAll(work)
	if err := s.finishImport(w, id, stage, importMetadata{Kind: "zip"}); err != nil {
		writeValidationError(w, err)
		return
	}
	complete = true
}

func (s *Server) prepareGitImport(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var req struct {
		RepositoryURL string `json:"repositoryUrl"`
		Ref           string `json:"ref"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 16385)).Decode(&req) != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "JSONが不正です", "body")
		return
	}
	if err := ValidateRepositoryURL(req.RepositoryURL); err != nil {
		writeValidationError(w, err)
		return
	}
	req.RepositoryURL, req.Ref = RepositoryURLAndRef(req.RepositoryURL, req.Ref)
	if err := ValidateRef(req.Ref); err != nil {
		writeValidationError(w, err)
		return
	}
	id, stage, err := s.newImportStage()
	if err != nil {
		writeAPIError(w, 500, "IMPORT_FAILED", "取り込み領域を作成できません", "")
		return
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(stage)
		}
	}()
	runner := s.SourceRunner
	if runner == nil {
		runner = OSRunner{}
	}
	if _, err := runner.Run(r.Context(), "git", "clone", "--no-tags", "--depth", "1", "--branch", req.Ref, req.RepositoryURL, filepath.Join(stage, "source")); err != nil {
		writeAPIError(w, 400, "IMPORT_FAILED", "GitHubリポジトリを取得できません", "repositoryUrl")
		return
	}
	if err := s.finishImport(w, id, stage, importMetadata{Kind: "github", RepositoryURL: req.RepositoryURL, Ref: req.Ref}); err != nil {
		writeValidationError(w, err)
		return
	}
	complete = true
}

func (s *Server) newImportStage() (string, string, error) {
	if s.AppsRoot == "" {
		return "", "", fmt.Errorf("アプリ領域がありません")
	}
	s.importMu.Lock()
	defer s.importMu.Unlock()
	root := filepath.Join(s.AppsRoot, ".imports")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", "", err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || ValidateRequestID(entry.Name()) != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", "", err
		}
		if time.Since(info.ModTime()) > 24*time.Hour {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return "", "", err
			}
			continue
		}
		count++
	}
	if count >= 20 {
		return "", "", fmt.Errorf("未完了の取り込みが多すぎます")
	}
	id := uuid.NewString()
	stage := filepath.Join(root, id)
	if err := os.Mkdir(stage, 0700); err != nil {
		return "", "", err
	}
	return id, stage, nil
}

func (s *Server) finishImport(w http.ResponseWriter, id, stage string, meta importMetadata) error {
	inspection, err := InspectImport(filepath.Join(stage, "source"))
	if err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "metadata.json"), data, 0600); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"importId": id, "manifest": inspection.Manifest, "composeFiles": inspection.ComposeFiles})
	return nil
}

func (s *Server) discardImport(w http.ResponseWriter, id string) {
	if ValidateRequestID(id) != nil || s.AppsRoot == "" {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "importIdが不正です", "importId")
		return
	}
	if err := os.RemoveAll(filepath.Join(s.AppsRoot, ".imports", id)); err != nil {
		writeAPIError(w, 500, "IMPORT_FAILED", "取り込み内容を破棄できません", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createImportedApplication(w http.ResponseWriter, r *http.Request, req createApplicationInput) {
	if ValidateRequestID(req.RequestID) != nil || ValidateRequestID(req.ImportID) != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "requestIdとimportIdはUUIDで指定してください", "importId")
		return
	}
	if req.RepositoryURL != "" || req.Ref != "" {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "取り込み済みsourceにはURLとrefを指定できません", "importId")
		return
	}
	if err := ValidatePublishedSubdomain(req.Subdomain, s.BaseDomain); err != nil {
		writeValidationError(w, err)
		return
	}
	serialized, _ := json.Marshal(req)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(serialized))
	var existingID, kind, oldFingerprint string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,kind,request_fingerprint FROM operations WHERE request_id=?`, req.RequestID).Scan(&existingID, &kind, &oldFingerprint)
	if err == nil {
		if kind != "CREATE" || oldFingerprint != fingerprint {
			writeAPIError(w, 400, "REQUEST_ID_REUSED", "requestIdが異なる要求に再利用されています", "requestId")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"name": "operations/" + existingID})
		return
	}
	if err != sql.ErrNoRows {
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "Operationを確認できません", "")
		return
	}
	stage := filepath.Join(s.AppsRoot, ".imports", req.ImportID)
	metaData, err := os.ReadFile(filepath.Join(stage, "metadata.json"))
	var meta importMetadata
	if err != nil || json.Unmarshal(metaData, &meta) != nil || (meta.Kind != "github" && meta.Kind != "zip") {
		writeAPIError(w, 400, "INVALID_IMPORT", "取り込み内容が見つかりません。もう一度確認してください", "importId")
		return
	}
	selection := ImportSelection{ComposeFile: req.ComposeFile, OverrideFiles: req.OverrideFiles, DisplayName: req.DisplayName, Description: req.Description, PublicService: req.PublicService, PublicPort: req.PublicPort}
	if err := ValidateImportedSelection(filepath.Join(stage, "source"), selection); err != nil {
		writeValidationError(w, err)
		return
	}
	appID := uuid.NewString()
	appRoot := filepath.Join(s.AppsRoot, appID)
	if err := os.MkdirAll(appRoot, 0700); err != nil {
		writeAPIError(w, 500, "IMPORT_FAILED", "アプリ領域を作成できません", "")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		_ = os.RemoveAll(appRoot)
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "アプリ登録を開始できません", "")
		return
	}
	defer tx.Rollback()
	overrides, _ := json.Marshal(req.OverrideFiles)
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO applications(id,subdomain,repository_url,git_ref,source_kind,compose_file,override_files,manifest_name,manifest_description,manifest_service,manifest_port,public_service,public_port,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'),datetime('now'))`, appID, req.Subdomain, meta.RepositoryURL, meta.Ref, meta.Kind, req.ComposeFile, string(overrides), req.DisplayName, req.Description, req.PublicService, req.PublicPort, req.PublicService, req.PublicPort); err != nil {
		_ = os.RemoveAll(appRoot)
		writeAPIError(w, 409, "ALREADY_EXISTS", "同じsubdomainのアプリが既に存在します", "subdomain")
		return
	}
	op, err := createOperationWithExecutor(r.Context(), tx, appID, req.RequestID, "CREATE", fingerprint, `{"prepared":true}`)
	if err != nil {
		_ = os.RemoveAll(appRoot)
		writeAPIError(w, 409, "CONFLICT", "登録Operationを作成できません", "")
		return
	}
	pending := filepath.Join(appRoot, "pending-"+op.ID)
	if err := os.Rename(stage, pending); err != nil {
		_ = os.RemoveAll(appRoot)
		writeAPIError(w, 409, "CONFLICT", "取り込み内容は既に使用されています", "importId")
		return
	}
	if err := tx.Commit(); err != nil {
		_ = os.Rename(pending, stage)
		_ = os.RemoveAll(appRoot)
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "アプリ登録を確定できません", "")
		return
	}
	if s.worker != nil {
		if err := s.worker.Enqueue(op); err != nil {
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", err.Error())
			writeAPIError(w, 409, "CONFLICT", err.Error(), "")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"name": "operations/" + op.ID})
}

func (s *Server) updateApplicationImport(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || s.DB.Ping() != nil {
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	if !requireJSON(w, r) {
		return
	}
	var req updateImportInput
	if json.NewDecoder(r.Body).Decode(&req) != nil || ValidateRequestID(req.RequestID) != nil || ValidateRequestID(req.ImportID) != nil || ValidateRequestID(appID(r)) != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "更新内容が不正です", "body")
		return
	}
	serialized, _ := json.Marshal(req)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(serialized))
	var existingID, existingApp, kind, oldFingerprint string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,application_id,kind,request_fingerprint FROM operations WHERE request_id=?`, req.RequestID).Scan(&existingID, &existingApp, &kind, &oldFingerprint)
	if err == nil {
		if existingApp != appID(r) || kind != "SYNC" || oldFingerprint != fingerprint {
			writeAPIError(w, 400, "REQUEST_ID_REUSED", "requestIdが異なる要求に再利用されています", "requestId")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"name": "operations/" + existingID})
		return
	}
	if err != sql.ErrNoRows {
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "Operationを確認できません", "")
		return
	}
	var sourceKind, state string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT source_kind,registration_state FROM applications WHERE id=?`, appID(r)).Scan(&sourceKind, &state); err != nil {
		writeAPIError(w, 404, "NOT_FOUND", "アプリが見つかりません", "application")
		return
	}
	if sourceKind != "zip" || state == "UNREGISTERED" {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "ZIPで登録されたアプリにだけ更新できます", "application")
		return
	}
	stage := filepath.Join(s.AppsRoot, ".imports", req.ImportID)
	data, err := os.ReadFile(filepath.Join(stage, "metadata.json"))
	var meta importMetadata
	if err != nil || json.Unmarshal(data, &meta) != nil || meta.Kind != "zip" {
		writeAPIError(w, 400, "INVALID_IMPORT", "ZIPの取り込み内容が見つかりません", "importId")
		return
	}
	if err := ValidateImportedSelection(filepath.Join(stage, "source"), req.ImportSelection); err != nil {
		writeValidationError(w, err)
		return
	}
	payload, _ := json.Marshal(struct {
		Prepared  bool            `json:"prepared"`
		Selection ImportSelection `json:"selection"`
	}{true, req.ImportSelection})
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "更新を開始できません", "")
		return
	}
	defer tx.Rollback()
	op, err := createOperationWithExecutor(r.Context(), tx, appID(r), req.RequestID, "SYNC", fingerprint, string(payload))
	if err != nil {
		writeAPIError(w, 409, "CONFLICT", "更新Operationを作成できません", "")
		return
	}
	pending := filepath.Join(s.AppsRoot, appID(r), "pending-"+op.ID)
	if err := os.Rename(stage, pending); err != nil {
		writeAPIError(w, 409, "CONFLICT", "取り込み内容は既に使用されています", "importId")
		return
	}
	if err := tx.Commit(); err != nil {
		_ = os.Rename(pending, stage)
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "更新を確定できません", "")
		return
	}
	if s.worker != nil {
		if err := s.worker.Enqueue(op); err != nil {
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", err.Error())
			writeAPIError(w, 409, "CONFLICT", err.Error(), "")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"name": "operations/" + op.ID})
}

// InspectImportはsourceを実行せず、登録画面に必要な候補を読み取る。
func InspectImport(root string) (SourceInspection, error) {
	result := SourceInspection{ComposeFiles: []ImportComposeFile{}}
	if err := ValidateSourceTree(root); err != nil {
		return result, err
	}
	manifest, err := readSourceManifest(root)
	if err != nil {
		return result, err
	}
	result.Manifest = manifest
	entries, err := os.ReadDir(root)
	if err != nil {
		return result, fmt.Errorf("sourceを読み取れません")
	}
	for _, entry := range entries {
		if entry.IsDir() || !containsComposeFile(entry.Name()) {
			continue
		}
		candidate := ImportComposeFile{Name: entry.Name(), Services: []string{}}
		data, err := readComposeFile(root, entry.Name())
		if err != nil {
			return result, fmt.Errorf("Composeファイルを読み取れません")
		}
		var model struct {
			Services map[string]any `yaml:"services"`
		}
		if yaml.Unmarshal(data, &model) == nil {
			for service := range model.Services {
				candidate.Services = append(candidate.Services, service)
			}
			sort.Strings(candidate.Services)
			interfaces, _ := ComposeWebInterfaces([]ComposeSource{{Data: data}})
			candidate.Interfaces = interfaces
		}
		if candidate.Interfaces == nil {
			candidate.Interfaces = []WebInterface{}
		}
		result.ComposeFiles = append(result.ComposeFiles, candidate)
	}
	if len(result.ComposeFiles) == 0 {
		return result, fmt.Errorf("使用できるComposeファイルが見つかりません")
	}
	return result, nil
}
