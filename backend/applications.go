package backend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) applicationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/applications/{application}", s.getApplication)
	m.HandleFunc("PATCH /api/v1/applications/{application}", s.patchApplication)
	m.HandleFunc("DELETE /api/v1/applications/{application}", s.deleteApplication)
	m.HandleFunc("POST /api/v1/applications/", s.appOperation)
	m.HandleFunc("GET /api/v1/applications/{application}/configuration", s.getConfiguration)
	m.HandleFunc("PATCH /api/v1/applications/{application}/configuration", s.patchConfiguration)
	m.HandleFunc("GET /api/v1/operations/", s.operationRoute)
}
func (s *Server) getConfiguration(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT name,is_secret FROM application_variables WHERE application_id=?`, appID(r))
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "設定を取得できません", "")
		return
	}
	defer rows.Close()
	v := []map[string]any{}
	for rows.Next() {
		var n string
		var secret int
		_ = rows.Scan(&n, &secret)
		v = append(v, map[string]any{"name": n, "isSecret": secret != 0})
	}
	writeJSON(w, 200, map[string]any{"variables": v})
}
func (s *Server) patchConfiguration(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || s.DB.Ping() != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	if r.Header.Get("Content-Type") != "application/json" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json;") {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CONTENT_TYPE", "状態を変更する要求はJSONで指定してください", "contentType")
		return
	}
	var req struct {
		Variables map[string]struct {
			Value  string `json:"value"`
			Secret bool   `json:"secret"`
		} `json:"variables"`
		RequestID string `json:"requestId"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "JSONが不正です", "body")
		return
	}
	if err := ValidateRequestID(req.RequestID); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "requestIdはUUIDで指定してください", "requestId")
		return
	}
	type variableValue struct {
		name   string
		value  any
		secret bool
	}
	prepared := make([]variableValue, 0, len(req.Variables))
	for n, v := range req.Variables {
		if err := ValidateVariableName(n); err != nil {
			writeValidationError(w, err)
			return
		}
		if v.Value == "" {
			writeAPIError(w, 400, "INVALID_ARGUMENT", "環境変数の値が必要です", n)
			return
		}
		if strings.ContainsRune(v.Value, '\x00') {
			writeAPIError(w, 400, "INVALID_ARGUMENT", "環境変数の値にNUL文字は指定できません", n)
			return
		}
		var stored any = v.Value
		if v.Secret {
			if len(s.SecretKey) == 0 {
				writeAPIError(w, http.StatusServiceUnavailable, "SECRET_KEY_UNAVAILABLE", "secretを保存できません", n)
				return
			}
			var err error
			stored, err = Encrypt(s.SecretKey, []byte(v.Value))
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "SECRET_ENCRYPTION_FAILED", "secretを保存できません", "")
				return
			}
		}
		prepared = append(prepared, variableValue{name: n, value: stored, secret: v.Secret})
	}
	payload, _ := json.Marshal(req.Variables)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(payload))
	var existingID, existingKind, existingFingerprint string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,kind,request_fingerprint FROM operations WHERE request_id=?`, req.RequestID).Scan(&existingID, &existingKind, &existingFingerprint)
	if err == nil {
		if existingKind != "CONFIGURE" || existingFingerprint == "" || existingFingerprint != fingerprint {
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
	op, err := CreateOperationWithFingerprint(r.Context(), s.DB, appID(r), req.RequestID, "CONFIGURE", fingerprint)
	if err != nil {
		writeAPIError(w, 409, "CONFLICT", err.Error(), "")
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "設定保存を開始できません")
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "設定を保存できません", "")
		return
	}
	for _, variable := range prepared {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO application_variables(application_id,name,value,is_secret,updated_at) VALUES(?,?,?,?,datetime('now')) ON CONFLICT(application_id,name) DO UPDATE SET value=excluded.value,is_secret=excluded.is_secret,updated_at=excluded.updated_at`, appID(r), variable.name, variable.value, variable.secret); err != nil {
			_ = tx.Rollback()
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "設定を保存できません")
			writeAPIError(w, http.StatusInternalServerError, "DATABASE_ERROR", "設定を保存できません", "")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "設定を保存できません")
		writeAPIError(w, http.StatusInternalServerError, "DATABASE_ERROR", "設定を保存できません", "")
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
func (s *Server) operationRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, ":watch") {
		s.watchOperation(w, r)
		return
	}
	s.getOperation(w, r)
}
func operationID(r *http.Request) string {
	id := r.PathValue("operation")
	if id != "" {
		return id
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	return strings.TrimSuffix(p, ":watch")
}
func appID(r *http.Request) string {
	id := r.PathValue("application")
	if id != "" {
		return id
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/applications/")
	if i := strings.IndexByte(p, ':'); i >= 0 {
		p = p[:i]
	}
	return strings.TrimSuffix(p, "/")
}
func (s *Server) getApplication(w http.ResponseWriter, r *http.Request) {
	var id, sub, repo, ref, desired, observed, state, created, updated string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,subdomain,repository_url,git_ref,desired_state,observed_state,registration_state,created_at,updated_at FROM applications WHERE id=?`, appID(r)).Scan(&id, &sub, &repo, &ref, &desired, &observed, &state, &created, &updated)
	if err == sql.ErrNoRows {
		writeAPIError(w, 404, "NOT_FOUND", "アプリが見つかりません", "application")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "アプリを取得できません", "")
		return
	}
	var latestOperation string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM operations WHERE application_id=? AND state IN ('QUEUED','RUNNING') ORDER BY created_at DESC LIMIT 1`, id).Scan(&latestOperation); err != nil && err != sql.ErrNoRows {
		writeAPIError(w, 500, "DATABASE_ERROR", "Operation状態を取得できません", "")
		return
	}
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256([]byte(id+"\x00"+updated+"\x00"+desired+"\x00"+observed)))
	response := map[string]any{
		"name":              "applications/" + id,
		"subdomain":         sub,
		"repositoryUrl":     repo,
		"ref":               ref,
		"desiredState":      desired,
		"observedState":     observed,
		"registrationState": state,
		"createdAt":         created,
		"updatedAt":         updated,
		"reconciling":       latestOperation != "",
		"latestOperation":   latestOperation,
		"etag":              etag,
	}
	writeJSON(w, 200, response)
}
func (s *Server) patchApplication(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var p struct {
		Ref       string `json:"ref"`
		Subdomain string `json:"subdomain"`
		RequestID string `json:"requestId"`
	}
	if json.NewDecoder(r.Body).Decode(&p) != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "JSONが不正です", "body")
		return
	}
	if p.Ref == "" && p.Subdomain == "" {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "変更項目が必要です", "updateMask")
		return
	}
	if err := ValidateRequestID(p.RequestID); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "requestIdはUUIDで指定してください", "requestId")
		return
	}
	if p.Ref != "" {
		if err := ValidateRef(p.Ref); err != nil {
			writeValidationError(w, err)
			return
		}
	}
	if p.Subdomain != "" {
		if err := ValidateSubdomain(p.Subdomain); err != nil {
			writeValidationError(w, err)
			return
		}
	}
	payload, _ := json.Marshal(p)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(payload))
	op, err := CreateOperationWithPayload(r.Context(), s.DB, appID(r), p.RequestID, "UPDATE", fingerprint, string(payload))
	if err != nil {
		writeAPIError(w, 409, "CONFLICT", err.Error(), "")
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
func (s *Server) deleteApplication(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	op, err := s.makeAppOp(r, "UNREGISTER")
	if err != nil {
		if _, ok := err.(*ConflictError); ok {
			writeAPIError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
		} else {
			writeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), "requestId")
		}
		return
	}
	writeJSON(w, 202, map[string]string{"name": "operations/" + op.ID})
}
func (s *Server) appOperation(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	kind := "OPERATION"
	for _, name := range []string{"start", "stop", "sync", "rebuild", "purge"} {
		if strings.HasSuffix(r.URL.Path, ":"+name) {
			kind = strings.ToUpper(name)
		}
	}
	op, err := s.makeAppOp(r, kind)
	if err != nil {
		if _, ok := err.(*ConflictError); ok {
			writeAPIError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
		} else {
			writeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), "requestId")
		}
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
func (s *Server) makeAppOp(r *http.Request, kind string) (Operation, error) {
	id := appID(r)
	var exists string
	clause := " AND registration_state='ACTIVE'"
	if strings.HasSuffix(r.URL.Path, ":purge") {
		clause = ""
	}
	if err := s.DB.QueryRowContext(context.Background(), `SELECT id FROM applications WHERE id=?`+clause, id).Scan(&exists); err != nil {
		return Operation{}, err
	}
	var request struct {
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return Operation{}, errors.New("requestIdはJSON bodyで指定してください")
	}
	if err := ValidateRequestID(request.RequestID); err != nil {
		return Operation{}, errors.New("requestIdはUUIDで指定してください")
	}
	return CreateOperation(r.Context(), s.DB, id, request.RequestID, kind)
}
func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	var o Operation
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,application_id,request_id,kind,state,error_message FROM operations WHERE id=?`, operationID(r)).Scan(&o.ID, &o.ApplicationID, &o.RequestID, &o.Kind, &o.State, &o.ErrorMessage)
	if err == sql.ErrNoRows {
		writeAPIError(w, 404, "NOT_FOUND", "Operationが見つかりません", "operation")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "Operationを取得できません", "")
		return
	}
	writeJSON(w, 200, map[string]string{"name": "operations/" + o.ID, "state": strings.ToLower(o.State), "errorMessage": o.ErrorMessage})
}
func (s *Server) watchOperation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	ch := s.events.Subscribe(operationID(r))
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			b, _ := json.Marshal(e.Data)
			_, _ = w.Write([]byte("id: " + e.ID + "\nevent: " + e.Type + "\ndata: " + string(b) + "\n\n"))
			f.Flush()
		}
	}
}
