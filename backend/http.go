package backend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/google/uuid"
)

type Server struct {
	DB            *sql.DB
	SecretKey     []byte
	AllowedHost   string
	AllowedOrigin string
	Ready         func() bool
	events        *Events
	worker        *Worker
	Logs          *LogStore
}

func NewServer(db *sql.DB, run func(context.Context, Operation) error) *Server {
	s := &Server{DB: db, events: NewEvents(), Logs: NewLogStore(db)}
	s.worker = NewWorker(db, run)
	s.worker.events = s.events
	s.worker.logs = s.Logs
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.events == nil {
		s.events = NewEvents()
	}
	api := generatedAPI{server: s}
	wrapper := ServerInterfaceWrapper{Handler: api}
	// GoのServeMuxはwildcard直後のcustom method（{application}:start）を
	// patternとして受け付けないため、生成wrapperの通常routeと分離する。
	mux.HandleFunc("GET /api/v1/health/live", wrapper.HealthLive)
	mux.HandleFunc("GET /api/v1/health/ready", wrapper.HealthReady)
	mux.HandleFunc("GET /api/v1/applications", wrapper.ListApplications)
	mux.HandleFunc("POST /api/v1/applications", wrapper.CreateApplication)
	mux.HandleFunc("DELETE /api/v1/applications/{application}", wrapper.UnregisterApplication)
	// 通常取得とcustom method suffixは同じprefix dispatcherで分岐する。
	mux.HandleFunc("GET /api/v1/applications/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logEntries:watch") {
			s.watchLogEntries(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/logEntries") {
			s.listLogEntries(w, r)
			return
		}
		s.getApplication(w, r)
	})
	mux.HandleFunc("PATCH /api/v1/applications/{application}", wrapper.UpdateApplication)
	mux.HandleFunc("GET /api/v1/applications/{application}/configuration", wrapper.GetConfiguration)
	mux.HandleFunc("PATCH /api/v1/applications/{application}/configuration", wrapper.UpdateConfiguration)
	// custom method suffix（{operation}:watch）はwildcardの末尾制約により
	// 生成wrapperへ直接登録できないため、prefix dispatcherで振り分ける。
	mux.HandleFunc("GET /api/v1/operations/", s.operationRoute)
	// custom methodとSSE tail routeはURL suffixを検査する既存dispatcherへ委譲する。
	mux.HandleFunc("POST /api/v1/applications/", s.appOperation)
	return requestBoundary(openAPIValidation(mux), s.AllowedHost, s.AllowedOrigin)
}

func openAPIValidation(next http.Handler) http.Handler {
	doc, err := GetSwagger()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeAPIError(w, http.StatusInternalServerError, "OPENAPI_INVALID", "OpenAPI契約を読み込めません", "")
		})
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeAPIError(w, http.StatusInternalServerError, "OPENAPI_INVALID", "OpenAPI契約を検証できません", "")
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, params, err := router.FindRoute(r)
		if err == nil {
			input := &openapi3filter.RequestValidationInput{Request: r, PathParams: params, Route: route}
			if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				writeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "API要求がOpenAPI契約に適合しません", "body")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) listApplications(w http.ResponseWriter, _ *http.Request) {
	if s.DB == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	rows, err := s.DB.Query(`SELECT id, subdomain, repository_url, git_ref, desired_state, observed_state, registration_state, latest_error, updated_at FROM applications WHERE registration_state IN ('ACTIVE','UNREGISTERED') ORDER BY subdomain`)
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "アプリ一覧を取得できません", "")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sub, repo, ref, desired, observed, state, latestError, updated string
		if err := rows.Scan(&id, &sub, &repo, &ref, &desired, &observed, &state, &latestError, &updated); err != nil {
			writeAPIError(w, 500, "DATABASE_ERROR", "アプリ一覧を取得できません", "")
			return
		}
		var activeOperation, latestOperation string
		if err := s.DB.QueryRow(`SELECT id FROM operations WHERE application_id=? AND state IN ('QUEUED','RUNNING') ORDER BY created_at DESC LIMIT 1`, id).Scan(&activeOperation); err != nil && err != sql.ErrNoRows {
			writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Operation状態を取得できません", "")
			return
		}
		if err := s.DB.QueryRow(`SELECT id FROM operations WHERE application_id=? ORDER BY created_at DESC LIMIT 1`, id).Scan(&latestOperation); err != nil && err != sql.ErrNoRows {
			writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Operation状態を取得できません", "")
			return
		}
		items = append(items, map[string]any{
			"name": "applications/" + id, "subdomain": sub, "repositoryUrl": repo, "ref": ref,
			"desiredState": desired, "observedState": observed, "registrationState": state,
			"observedAt": time.Now().UTC().Format(time.RFC3339Nano), "reconciling": activeOperation != "",
			"latestOperation": operationResourceName(latestOperation), "etag": fmt.Sprintf("\"%x\"", sha256.Sum256([]byte(id+"\x00"+updated+"\x00"+desired+"\x00"+observed))),
			"latestError": latestError,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"applications": items})
}

func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || s.DB.Ping() != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
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
	if err := ValidateRef(req.Ref); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "refが不正です", "ref")
		return
	}
	if err := ValidateRequestID(req.RequestID); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "requestIdはUUIDで指定してください", "requestId")
		return
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(req.RepositoryURL+"\n"+req.Ref+"\n"+req.Subdomain)))
	var existingID, existingKind, existingFingerprint string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,kind,request_fingerprint FROM operations WHERE request_id=?`, req.RequestID).Scan(&existingID, &existingKind, &existingFingerprint)
	if err == nil {
		if existingKind != "CREATE" || existingFingerprint == "" || existingFingerprint != fingerprint {
			writeAPIError(w, http.StatusBadRequest, "REQUEST_ID_REUSED", "requestIdが異なる要求に再利用されています", "requestId")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"name": "operations/" + existingID})
		return
	}
	if err != sql.ErrNoRows {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Operationを確認できません", "")
		return
	}
	// アプリとOperationを同一transactionで作成し、片方だけが残る状態を防ぐ。
	id := uuid.NewString()
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "アプリ登録を開始できません", "")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO applications(id,subdomain,repository_url,git_ref,created_at,updated_at) VALUES(?,?,?,?,datetime('now'),datetime('now'))`, id, req.Subdomain, req.RepositoryURL, req.Ref); err != nil {
		_ = tx.Rollback()
		writeAPIError(w, 409, "ALREADY_EXISTS", "同じsubdomainのアプリが既に存在します", "subdomain")
		return
	}
	op, err := createOperationWithExecutor(r.Context(), tx, id, req.RequestID, "CREATE", fingerprint, "")
	if err != nil {
		_ = tx.Rollback()
		if _, ok := err.(*ConflictError); ok {
			writeAPIError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
		} else {
			writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Operationを作成できません", "")
		}
		return
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "アプリ登録を確定できません", "")
		return
	}
	if s.worker != nil {
		if err := s.worker.Enqueue(op); err != nil {
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", err.Error())
			writeAPIError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
			return
		}
	}
	writeJSON(w, 202, map[string]string{"name": "operations/" + op.ID})
}

func requestBoundary(next http.Handler, allowedHost, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedHost != "" && r.Host != allowedHost {
			writeAPIError(w, http.StatusForbidden, "HOST_FORBIDDEN", "許可されていないHostです", "host")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != allowedOrigin {
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
	statusName := map[int]string{400: "INVALID_ARGUMENT", 403: "PERMISSION_DENIED", 404: "NOT_FOUND", 409: "ABORTED", 500: "INTERNAL", 503: "UNAVAILABLE"}[status]
	if statusName == "" {
		statusName = "UNKNOWN"
	}
	details := []any{map[string]any{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": reason, "domain": "labwebsystem"}}
	if status == http.StatusBadRequest && field != "" {
		details = append([]any{map[string]any{"@type": "type.googleapis.com/google.rpc.BadRequest", "fieldViolations": []any{map[string]string{"field": field, "description": message}}}}, details...)
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": status, "message": message, "status": statusName, "details": details}})
}

func writeValidationError(w http.ResponseWriter, err error) {
	if v, ok := err.(*ValidationError); ok {
		writeAPIError(w, 400, v.Reason, v.Message, v.Field)
		return
	}
	writeAPIError(w, 400, "INVALID_ARGUMENT", err.Error(), "")
}

func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" || strings.HasPrefix(contentType, "application/json;") {
		return true
	}
	writeAPIError(w, http.StatusBadRequest, "INVALID_CONTENT_TYPE", "状態を変更する要求はJSONで指定してください", "contentType")
	return false
}
