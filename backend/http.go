package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type Server struct {
	DB     *sql.DB
	Ready  func() bool
	events *Events
	worker *Worker
}

func NewServer(db *sql.DB, run func(context.Context, Operation) error) *Server {
	s := &Server{DB: db, events: NewEvents()}
	s.worker = NewWorker(db, run)
	s.worker.events = s.events
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		ready := s.DB != nil
		if s.Ready != nil {
			ready = ready && s.Ready()
		}
		if ready {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		} else {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		}
	})
	mux.HandleFunc("GET /api/v1/applications", s.listApplications)
	mux.HandleFunc("POST /api/v1/applications", s.createApplication)
	s.applicationRoutes(mux)
	if s.events == nil {
		s.events = NewEvents()
	}
	return noCORS(mux)
}

func (s *Server) listApplications(w http.ResponseWriter, _ *http.Request) {
	if s.DB == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	rows, err := s.DB.Query(`SELECT id, subdomain, repository_url, git_ref, desired_state, observed_state, registration_state FROM applications WHERE registration_state='ACTIVE' ORDER BY subdomain`)
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "アプリ一覧を取得できません", "")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, sub, repo, ref, desired, observed, state string
		if err := rows.Scan(&id, &sub, &repo, &ref, &desired, &observed, &state); err != nil {
			writeAPIError(w, 500, "DATABASE_ERROR", "アプリ一覧を取得できません", "")
			return
		}
		items = append(items, map[string]string{"name": "applications/" + id, "subdomain": sub, "repositoryUrl": repo, "ref": ref, "desiredState": desired, "observedState": observed, "registrationState": state})
	}
	writeJSON(w, http.StatusOK, map[string]any{"applications": items})
}

func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json;") {
		writeAPIError(w, 400, "INVALID_CONTENT_TYPE", "状態を変更する要求はJSONで指定してください", "contentType")
		return
	}
	var req struct {
		RepositoryURL string `json:"repositoryUrl"`
		Ref           string `json:"ref"`
		Subdomain     string `json:"subdomain"`
		RequestID     string `json:"requestId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "JSONが不正です", "body")
		return
	}
	if err := ValidateRepositoryURL(req.RepositoryURL); err != nil {
		writeValidationError(w, err)
		return
	}
	if err := ValidateSubdomain(req.Subdomain); err != nil {
		writeValidationError(w, err)
		return
	}
	if req.Ref == "" {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "refが必要です", "ref")
		return
	}
	if _, err := uuid.Parse(req.RequestID); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "requestIdはUUIDで指定してください", "requestId")
		return
	}
	if s.DB == nil {
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	// 登録処理はOperationを先に永続化し、取得・検証・起動はworkerへ委譲する。
	id := uuid.NewString()
	if _, err := s.DB.ExecContext(context.Background(), `INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES(?,?,?,?,datetime('now'),datetime('now'))`, id, req.Subdomain, req.RepositoryURL, req.Ref); err != nil {
		writeAPIError(w, 409, "ALREADY_EXISTS", "同じsubdomainのアプリが既に存在します", "subdomain")
		return
	}
	op, err := CreateOperation(r.Context(), s.DB, id, req.RequestID, "CREATE")
	if err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", err.Error(), "requestId")
		return
	}
	if s.worker != nil {
		_ = s.worker.Enqueue(op)
	}
	writeJSON(w, 202, map[string]string{"name": "operations/" + op.ID})
}

func noCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			writeAPIError(w, http.StatusForbidden, "ORIGIN_FORBIDDEN", "許可されていないOriginです", "origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeAPIError(w http.ResponseWriter, status int, reason, message, field string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": status, "message": message, "status": "INVALID_ARGUMENT", "details": []any{map[string]any{"@type": "type.googleapis.com/google.rpc.BadRequest", "fieldViolations": []any{map[string]string{"field": field, "description": message}}}, map[string]any{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": reason, "domain": "labwebsystem"}}}})
}

func writeValidationError(w http.ResponseWriter, err error) {
	if v, ok := err.(*ValidationError); ok {
		writeAPIError(w, 400, v.Reason, v.Message, v.Field)
		return
	}
	writeAPIError(w, 400, "INVALID_ARGUMENT", err.Error(), "")
}
