package backend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type configurationVariableValue struct {
	name   string
	value  any
	secret bool
	keep   bool
}

func (s *Server) applicationRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/applications/{application}", s.getApplication)
	m.HandleFunc("PATCH /api/v1/applications/{application}", s.patchApplication)
	m.HandleFunc("DELETE /api/v1/applications/{application}", s.deleteApplication)
	m.HandleFunc("POST /api/v1/applications/", s.appOperation)
	m.HandleFunc("GET /api/v1/applications/{application}/configuration", s.getConfiguration)
	m.HandleFunc("PATCH /api/v1/applications/{application}/configuration", s.patchConfiguration)
	m.HandleFunc("GET /api/v1/operations/", s.operationRoute)
}

// resourcePoolRoutes intentionally expose only LWS-owned resources.  Compose source paths
// are never accepted as an allocation; device creation must select a detected candidate.
func (s *Server) resourcePoolRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/resource-pools", s.getResourcePools)
	m.HandleFunc("POST /api/v1/resource-pools/devices", s.createPoolDevice)
}

func (s *Server) getResourcePools(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeAPIError(w, 503, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	_, _ = s.refreshLWSDevices(r.Context())
	includeSystem := r.URL.Query().Get("includeSystem") == "true"
	candidates := []PhysicalDevice{}
	if s.DeviceScanner != nil {
		candidates, _ = s.DeviceScanner.Scan(r.Context(), includeSystem)
	}
	devices := []map[string]any{}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id,name,stable_id,current_path,status,metadata FROM lws_devices ORDER BY name`)
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "デバイスプールを取得できません", "")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, stable, path, status, metadata string
		if rows.Scan(&id, &name, &stable, &path, &status, &metadata) == nil {
			devices = append(devices, map[string]any{"id": id, "name": name, "stableId": stable, "currentPath": path, "status": strings.ToLower(status), "metadata": json.RawMessage(metadata)})
		}
	}
	volumes, networks := []map[string]any{}, []map[string]any{}
	apps, err := s.DB.QueryContext(r.Context(), `SELECT id,subdomain,compose_file,override_files FROM applications WHERE registration_state IN ('ACTIVE','CONFIGURING') ORDER BY subdomain`)
	if err == nil {
		defer apps.Close()
		for apps.Next() {
			var id, sub, composeFile, overrideFilesJSON string
			if apps.Scan(&id, &sub, &composeFile, &overrideFilesJSON) != nil {
				continue
			}
			if s.AppsRoot != "" {
				if data, e := os.ReadFile(filepath.Join(s.AppsRoot, id, "source", composeFile)); e == nil {
					if names, e := managedComposeVolumeNames(id, s.AppsRoot, data, overrideFilesJSON); e == nil {
						for _, n := range names {
							inUse, status, deletable := s.volumeState(r.Context(), n)
							volumes = append(volumes, map[string]any{"name": n, "application": "applications/" + id, "applicationName": sub, "status": status, "inUse": inUse, "deletable": deletable})
						}
					}
					if names, e := ComposeNetworkNames(data, ProjectName(id)); e == nil {
						for _, name := range names {
							networks = append(networks, map[string]any{"name": name, "application": "applications/" + id, "applicationName": sub, "status": "managed", "kind": "compose"})
						}
					}
				}
			}
			networks = append(networks, map[string]any{"name": EdgeNetworkName(id), "application": "applications/" + id, "applicationName": sub, "status": "managed", "kind": "edge"})
		}
	}
	writeJSON(w, 200, map[string]any{"devices": devices, "physicalDevices": candidates, "volumes": volumes, "networks": networks})
}

func (s *Server) volumeState(ctx context.Context, name string) (bool, string, bool) {
	if s.Docker == nil {
		return true, "使用状況を確認できません", false
	}
	inUse, err := s.Docker.VolumeInUse(ctx, name)
	if err != nil {
		return true, "使用状況を確認できません", false
	}
	if inUse {
		return true, "使用中", false
	}
	return false, "未使用", true
}

func (s *Server) deleteResourcePoolVolume(w http.ResponseWriter, r *http.Request, volume string) {
	if s.Docker == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "RESOURCE_UNAVAILABLE", "Volumeを削除できる状態ではありません", "")
		return
	}
	inUse, err := s.Docker.VolumeInUse(r.Context(), volume)
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "VOLUME_USAGE_UNKNOWN", "Volumeの使用状況を確認できません", "volume")
		return
	}
	if inUse {
		writeAPIError(w, http.StatusConflict, "VOLUME_IN_USE", "使用中のVolumeは削除できません", "volume")
		return
	}
	if err := s.Docker.RemoveOwnedVolume(r.Context(), volume); err != nil {
		if strings.Contains(err.Error(), "見つかりません") {
			writeAPIError(w, http.StatusNotFound, "VOLUME_NOT_FOUND", "LWS管理下のVolumeが見つかりません", "volume")
			return
		}
		writeAPIError(w, http.StatusConflict, "VOLUME_DELETE_FAILED", "Volumeを削除できません", "volume")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": volume})
}

func managedComposeVolumeNames(id, appsRoot string, base []byte, overrideFilesJSON string) ([]string, error) {
	names, err := NamedVolumeNames(base)
	if err != nil {
		return nil, err
	}
	var overrides []string
	_ = json.Unmarshal([]byte(overrideFilesJSON), &overrides)
	all := append([]AutoVolume{}, mustAutoVolumes(base, id)...)
	for _, file := range overrides {
		data, readErr := os.ReadFile(filepath.Join(appsRoot, id, "source", file))
		if readErr != nil {
			continue
		}
		all = append(all, mustAutoVolumes(data, id)...)
	}
	seen := map[string]bool{}
	result := []string{}
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	for _, volume := range all {
		if !seen[volume.Name] {
			seen[volume.Name] = true
			result = append(result, volume.Name)
		}
	}
	sort.Strings(result)
	return result, nil
}

func mustAutoVolumes(data []byte, id string) []AutoVolume {
	result, _ := AutoVolumeReplacements(data, id)
	return result
}

func (s *Server) refreshLWSDevices(ctx context.Context) ([]PhysicalDevice, error) {
	return RefreshLWSDevices(ctx, s.DB, s.DeviceScanner)
}

func (s *Server) createPoolDevice(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || !requireJSON(w, r) {
		return
	}
	var req struct {
		Name              string `json:"name"`
		CandidateStableID string `json:"candidateStableId"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.CandidateStableID) == "" || s.DeviceScanner == nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "デバイス名と検出済み物理デバイスの選択が必要です", "body")
		return
	}
	candidates, err := s.DeviceScanner.Scan(r.Context(), true)
	if err != nil {
		writeAPIError(w, 503, "DEVICE_SCAN_FAILED", "物理デバイスを検出できません", "candidateStableId")
		return
	}
	var selected *PhysicalDevice
	for i := range candidates {
		if candidates[i].StableID == req.CandidateStableID {
			selected = &candidates[i]
			break
		}
	}
	if selected == nil {
		writeAPIError(w, 400, "DEVICE_NOT_FOUND", "選択された物理デバイスは現在接続されていません", "candidateStableId")
		return
	}
	id := "device-" + uuid.NewString()
	metadata, _ := json.Marshal(selected.Metadata)
	if _, err := s.DB.ExecContext(r.Context(), `INSERT INTO lws_devices(id,name,stable_id,current_path,status,metadata,created_at,updated_at) VALUES(?,?,?,?,?,?,datetime('now'),datetime('now'))`, id, req.Name, selected.StableID, selected.CurrentPath, "CONNECTED", metadata); err != nil {
		writeAPIError(w, 409, "CONFLICT", "このデバイス名または安定識別子は既に登録されています", "name")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (s *Server) getConfiguration(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	var exists string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM applications WHERE id=?`, appID(r)).Scan(&exists); err == sql.ErrNoRows {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "アプリが見つかりません", "application")
		return
	} else if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "アプリを確認できません", "")
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT name,value,is_secret FROM application_variables WHERE application_id=?`, appID(r))
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "設定を取得できません", "")
		return
	}
	defer rows.Close()
	configured := map[string]bool{}
	v := []map[string]any{}
	for rows.Next() {
		var n string
		var value []byte
		var secret int
		_ = rows.Scan(&n, &value, &secret)
		configured[n] = true
		item := map[string]any{"name": n, "isSecret": secret != 0, "configured": true, "required": false, "hasDefault": false}
		if secret == 0 {
			item["value"] = string(value)
		}
		v = append(v, item)
	}
	volumes := []map[string]any{}
	attachments := []DeviceAttachment{}
	interfaces := []WebInterface{}
	manifestService, publicService := "", ""
	manifestPort, publicPort := 0, 0
	bindings := map[string]map[string]any{}
	if rows, err := s.DB.QueryContext(r.Context(), `SELECT service,target_path,device_id FROM application_device_bindings WHERE application_id=?`, appID(r)); err == nil {
		defer rows.Close()
		for rows.Next() {
			var service, target, id string
			if rows.Scan(&service, &target, &id) == nil {
				bindings[service+"\x00"+target] = map[string]any{"deviceId": id}
			}
		}
	}
	if s.AppsRoot != "" {
		_ = s.DB.QueryRow(`SELECT manifest_service,manifest_port,public_service,public_port FROM applications WHERE id=?`, appID(r)).Scan(&manifestService, &manifestPort, &publicService, &publicPort)
		sources, err := s.applicationComposeSources(appID(r))
		if err == nil {
			if variables, err := MergeComposeVariables(sources); err == nil {
				for _, variable := range variables {
					if configured[variable.Name] {
						for _, item := range v {
							if item["name"] == variable.Name {
								item["required"] = variable.Required
								item["hasDefault"] = variable.HasDefault
								if variable.HasDefault {
									item["defaultValue"] = variable.Default
								}
							}
						}
						continue
					}
					item := map[string]any{"name": variable.Name, "isSecret": false, "configured": false, "required": variable.Required, "hasDefault": variable.HasDefault}
					if variable.HasDefault {
						item["defaultValue"] = variable.Default
					}
					v = append(v, item)
				}
			}
			for _, source := range sources {
				if names, err := NamedVolumeNames(source.Data); err == nil {
					for _, name := range names {
						volumes = append(volumes, map[string]any{"name": name})
					}
				}
				if found, err := ComposeDeviceAttachments(source.Data); err == nil {
					attachments = append(attachments, found...)
				}
			}
			interfaces, _ = ComposeWebInterfaces(sources)
			if manifestService != "" && manifestPort > 0 && ComposeHasService(sources, manifestService) {
				found := false
				for _, iface := range interfaces {
					if iface.Service == manifestService && iface.Port == manifestPort {
						found = true
						break
					}
				}
				if !found {
					interfaces = append([]WebInterface{{Service: manifestService, Port: manifestPort}}, interfaces...)
				}
			}
		}
	}
	if publicService == "" {
		publicService = manifestService
	}
	if publicPort == 0 {
		publicPort = manifestPort
	}
	overrideCompose := ""
	if data, err := os.ReadFile(filepath.Join(s.AppsRoot, appID(r), "runtime", "lws.override.yaml")); err == nil {
		overrideCompose = string(data)
	}
	effectiveCompose := ""
	if data, err := os.ReadFile(filepath.Join(s.AppsRoot, appID(r), "runtime", "lws.effective.yaml")); err == nil {
		effectiveCompose = string(data)
	}
	sort.Slice(v, func(i, j int) bool { return v[i]["name"].(string) < v[j]["name"].(string) })
	writeJSON(w, 200, map[string]any{
		"variables":             v,
		"volumes":               volumes,
		"network":               map[string]any{"name": EdgeNetworkName(appID(r)), "purpose": "公開サービスとLWSのReverse Proxyだけを接続"},
		"lwsOverrideCompose":    overrideCompose,
		"effectiveCompose":      effectiveCompose,
		"manifestPublicService": manifestService,
		"manifestPublicPort":    manifestPort,
		"publicService":         publicService,
		"publicPort":            publicPort,
		"webInterfaces": func() []map[string]any {
			result := []map[string]any{}
			for _, iface := range interfaces {
				result = append(result, map[string]any{"service": iface.Service, "port": iface.Port})
			}
			return result
		}(),
		"devices": func() []map[string]any {
			result := []map[string]any{}
			for _, a := range attachments {
				item := map[string]any{"service": a.Service, "sourceHint": a.Source, "targetPath": a.Target, "permissions": a.Permissions, "configured": false}
				if b, ok := bindings[a.Service+"\x00"+a.Target]; ok {
					item["configured"] = true
					item["deviceId"] = b["deviceId"]
				}
				result = append(result, item)
			}
			return result
		}(),
		"deviceAccess": map[string]any{"allowed": len(attachments) > 0, "message": func() string {
			if len(attachments) == 0 {
				return "Composeはデバイスを要求していません"
			}
			return "ComposeのデバイスはLWSデバイスプールから設定します"
		}()},
		"ready": configurationReady(v, attachments, bindings),
	})
}

func (s *Server) applicationComposeSources(id string) ([]ComposeSource, error) {
	var composeFile, encoded string
	if err := s.DB.QueryRow(`SELECT compose_file,override_files FROM applications WHERE id=?`, id).Scan(&composeFile, &encoded); err != nil {
		return nil, err
	}
	var overrides []string
	if encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &overrides); err != nil {
			return nil, err
		}
	}
	return ReadComposeSources(filepath.Join(s.AppsRoot, id, "source"), composeFile, overrides)
}

func configurationReady(vars []map[string]any, devices []DeviceAttachment, bindings map[string]map[string]any) bool {
	for _, v := range vars {
		if required, _ := v["required"].(bool); required {
			configured, _ := v["configured"].(bool)
			if !configured {
				return false
			}
		}
	}
	for _, d := range devices {
		if _, ok := bindings[d.Service+"\x00"+d.Target]; !ok {
			return false
		}
	}
	return true
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
			Keep   bool   `json:"keep"`
		} `json:"variables"`
		RequestID      string `json:"requestId"`
		DeviceBindings []struct {
			Service    string `json:"service"`
			TargetPath string `json:"targetPath"`
			DeviceID   string `json:"deviceId"`
		} `json:"deviceBindings"`
		PublicService string `json:"publicService"`
		PublicPort    int    `json:"publicPort"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "JSONが不正です", "body")
		return
	}
	if err := ValidateRequestID(req.RequestID); err != nil {
		writeAPIError(w, 400, "INVALID_ARGUMENT", "requestIdはUUIDで指定してください", "requestId")
		return
	}
	if req.PublicService != "" || req.PublicPort != 0 {
		if req.PublicService == "" || req.PublicPort < 1 || req.PublicPort > 65535 {
			writeAPIError(w, 400, "INVALID_ARGUMENT", "公開Webインターフェースの指定が不正です", "publicService")
			return
		}
		sources, err := s.applicationComposeSources(appID(r))
		if err != nil {
			writeAPIError(w, 400, "INVALID_ARGUMENT", "Composeを読み取れません", "publicService")
			return
		}
		interfaces, err := ComposeWebInterfaces(sources)
		if err != nil {
			writeValidationError(w, err)
			return
		}
		valid := false
		for _, iface := range interfaces {
			if iface.Service == req.PublicService && iface.Port == req.PublicPort {
				valid = true
				break
			}
		}
		if !valid {
			var manifestService string
			var manifestPort int
			_ = s.DB.QueryRowContext(r.Context(), `SELECT manifest_service,manifest_port FROM applications WHERE id=?`, appID(r)).Scan(&manifestService, &manifestPort)
			valid = req.PublicService == manifestService && req.PublicPort == manifestPort && ComposeHasService(sources, manifestService)
		}
		if !valid {
			writeAPIError(w, 400, "INVALID_ARGUMENT", "公開WebインターフェースがComposeに存在しません", "publicService")
			return
		}
	}
	prepared := make([]configurationVariableValue, 0, len(req.Variables))
	if s.AppsRoot != "" {
		sources, err := s.applicationComposeSources(appID(r))
		if err == nil {
			attachments := []DeviceAttachment{}
			for _, source := range sources {
				found, _ := ComposeDeviceAttachments(source.Data)
				attachments = append(attachments, found...)
			}
			wanted := map[string]bool{}
			for _, a := range attachments {
				wanted[a.Service+"\x00"+a.Target] = true
			}
			seen := map[string]bool{}
			for _, b := range req.DeviceBindings {
				key := b.Service + "\x00" + b.TargetPath
				if !wanted[key] || seen[key] || b.DeviceID == "" {
					writeAPIError(w, 400, "INVALID_ARGUMENT", "Composeのdevice割り当てが不正です", "deviceBindings")
					return
				}
				seen[key] = true
				var status string
				if err := s.DB.QueryRowContext(r.Context(), `SELECT status FROM lws_devices WHERE id=?`, b.DeviceID).Scan(&status); err != nil || status != "CONNECTED" {
					writeAPIError(w, 400, "DEVICE_UNAVAILABLE", "選択されたLWSデバイスは接続されていません", "deviceBindings")
					return
				}
			}
		}
	}
	for n, v := range req.Variables {
		if err := ValidateVariableName(n); err != nil {
			writeValidationError(w, err)
			return
		}
		if v.Keep {
			if !v.Secret {
				writeAPIError(w, 400, "INVALID_ARGUMENT", "secret以外の環境変数は保持指定できません", n)
				return
			}
			prepared = append(prepared, configurationVariableValue{name: n, secret: true, keep: true})
			continue
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
		prepared = append(prepared, configurationVariableValue{name: n, value: stored, secret: v.Secret})
	}
	payload, _ := json.Marshal(struct {
		Variables map[string]struct {
			Value  string `json:"value"`
			Secret bool   `json:"secret"`
			Keep   bool   `json:"keep"`
		} `json:"variables"`
		PublicService string `json:"publicService"`
		PublicPort    int    `json:"publicPort"`
	}{req.Variables, req.PublicService, req.PublicPort})
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
		if variable.keep {
			var exists int
			if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM application_variables WHERE application_id=? AND name=? AND is_secret=1)`, appID(r), variable.name).Scan(&exists); err != nil || exists == 0 {
				_ = tx.Rollback()
				_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "secretの状態が変更されています")
				writeAPIError(w, http.StatusConflict, "CONFLICT", "secretの状態が変更されています。設定を読み込み直してください", variable.name)
				return
			}
			continue
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO application_variables(application_id,name,value,is_secret,updated_at) VALUES(?,?,?,?,datetime('now')) ON CONFLICT(application_id,name) DO UPDATE SET value=excluded.value,is_secret=excluded.is_secret,updated_at=excluded.updated_at`, appID(r), variable.name, variable.value, variable.secret); err != nil {
			_ = tx.Rollback()
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "設定を保存できません")
			writeAPIError(w, http.StatusInternalServerError, "DATABASE_ERROR", "設定を保存できません", "")
			return
		}
	}
	if req.PublicService != "" {
		if _, err := tx.ExecContext(r.Context(), `UPDATE applications SET public_service=?,public_port=?,updated_at=datetime('now') WHERE id=?`, req.PublicService, req.PublicPort, appID(r)); err != nil {
			_ = tx.Rollback()
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "公開先を保存できません")
			writeAPIError(w, 500, "DATABASE_ERROR", "公開先を保存できません", "publicService")
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM application_variables WHERE application_id=? AND name NOT IN (SELECT value FROM json_each(?))`, appID(r), variableNamesJSON(prepared)); err != nil {
		_ = tx.Rollback()
		_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", "設定を保存できません")
		writeAPIError(w, http.StatusInternalServerError, "DATABASE_ERROR", "設定を保存できません", "")
		return
	}
	// Omitted bindings mean that this is an environment-only save.  They must not
	// silently detach hardware selected in an earlier settings update.
	if req.DeviceBindings != nil {
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM application_device_bindings WHERE application_id=?`, appID(r)); err != nil {
			_ = tx.Rollback()
			writeAPIError(w, 500, "DATABASE_ERROR", "デバイス設定を保存できません", "")
			return
		}
		for _, b := range req.DeviceBindings {
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO application_device_bindings(application_id,service,target_path,device_id) VALUES(?,?,?,?)`, appID(r), b.Service, b.TargetPath, b.DeviceID); err != nil {
				_ = tx.Rollback()
				writeAPIError(w, 500, "DATABASE_ERROR", "デバイス設定を保存できません", "")
				return
			}
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

func variableNamesJSON(variables []configurationVariableValue) string {
	names := make([]string, 0, len(variables))
	for _, variable := range variables {
		names = append(names, variable.name)
	}
	data, _ := json.Marshal(names)
	return string(data)
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
func operationResourceName(id string) string {
	if id == "" {
		return ""
	}
	return "operations/" + id
}
func appID(r *http.Request) string {
	id := r.PathValue("application")
	if id != "" {
		return id
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/applications/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	if i := strings.IndexByte(p, ':'); i >= 0 {
		p = p[:i]
	}
	return strings.TrimSuffix(p, "/")
}
func (s *Server) getApplication(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	var id, sub, repo, ref, desired, observed, state, latestError, created, updated string
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,subdomain,repository_url,git_ref,desired_state,observed_state,registration_state,latest_error,created_at,updated_at FROM applications WHERE id=?`, appID(r)).Scan(&id, &sub, &repo, &ref, &desired, &observed, &state, &latestError, &created, &updated)
	if err == sql.ErrNoRows {
		writeAPIError(w, 404, "NOT_FOUND", "アプリが見つかりません", "application")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "アプリを取得できません", "")
		return
	}
	var activeOperation, latestOperation string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM operations WHERE application_id=? AND state IN ('QUEUED','RUNNING') ORDER BY created_at DESC LIMIT 1`, id).Scan(&activeOperation); err != nil && err != sql.ErrNoRows {
		writeAPIError(w, 500, "DATABASE_ERROR", "Operation状態を取得できません", "")
		return
	}
	if err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM operations WHERE application_id=? ORDER BY created_at DESC LIMIT 1`, id).Scan(&latestOperation); err != nil && err != sql.ErrNoRows {
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
		"reconciling":       activeOperation != "",
		"latestOperation":   operationResourceName(latestOperation),
		"latestError":       latestError,
		"etag":              etag,
		"observedAt":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeJSON(w, 200, response)
}
func (s *Server) patchApplication(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || s.DB.Ping() != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
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
	// requestId は再送識別子であり、実行内容の同一性には含めない。
	requestID := p.RequestID
	p.RequestID = ""
	payload, _ := json.Marshal(p)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(payload))
	op, err := CreateOperationWithPayload(r.Context(), s.DB, appID(r), requestID, "UPDATE", fingerprint, string(payload))
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
	if s.DB == nil || s.DB.Ping() != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
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
	if s.worker != nil {
		if err := s.worker.Enqueue(op); err != nil {
			_ = SetOperationState(r.Context(), s.DB, op.ID, "CANCELLED", err.Error())
			writeAPIError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
			return
		}
	}
	writeJSON(w, 202, map[string]string{"name": "operations/" + op.ID})
}
func (s *Server) appOperation(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || s.DB.Ping() != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	if !requireJSON(w, r) {
		return
	}
	kind := ""
	for _, name := range []string{"register", "start", "stop", "sync", "rebuild", "purge"} {
		if strings.HasSuffix(r.URL.Path, ":"+name) {
			kind = strings.ToUpper(name)
			break
		}
	}
	if kind == "" {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "Operationが見つかりません", "operation")
		return
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
	var registrationState string
	if err := s.DB.QueryRowContext(context.Background(), `SELECT registration_state FROM applications WHERE id=?`, id).Scan(&registrationState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Operation{}, errors.New("アプリが見つかりません")
		}
		return Operation{}, err
	}
	if kind == "REGISTER" && registrationState != "UNREGISTERED" {
		return Operation{}, errors.New("再登録は登録解除済みアプリだけに実行できます")
	}
	if kind == "UNREGISTER" {
		if registrationState != "ACTIVE" && registrationState != "CONFIGURING" {
			return Operation{}, errors.New("登録済みまたは設定待ちアプリだけ登録解除できます")
		}
	} else if kind != "REGISTER" && kind != "PURGE" && registrationState != "ACTIVE" && !(kind == "START" && registrationState == "CONFIGURING") && !(kind == "SYNC" && registrationState == "CONFIGURING") && !(kind == "REBUILD" && registrationState == "CONFIGURING") {
		if registrationState == "CONFIGURING" {
			return Operation{}, errors.New("設定待ちアプリにはこの操作を実行できません。設定を保存してから開始してください")
		}
		return Operation{}, errors.New("登録解除済みアプリにはこの操作を実行できません")
	}
	var request struct {
		RequestID string `json:"requestId"`
		Confirm   *bool  `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return Operation{}, errors.New("requestIdはJSON bodyで指定してください")
	}
	if err := ValidateRequestID(request.RequestID); err != nil {
		return Operation{}, errors.New("requestIdはUUIDで指定してください")
	}
	if kind == "PURGE" && (request.Confirm == nil || !*request.Confirm) {
		return Operation{}, errors.New("purgeにはconfirm:trueが必要です")
	}
	return CreateOperation(r.Context(), s.DB, id, request.RequestID, kind)
}
func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	var o Operation
	err := s.DB.QueryRowContext(r.Context(), `SELECT id,application_id,request_id,kind,state,error_message,phase,display_message,created_at,updated_at FROM operations WHERE id=?`, operationID(r)).Scan(&o.ID, &o.ApplicationID, &o.RequestID, &o.Kind, &o.State, &o.ErrorMessage, &o.Phase, &o.DisplayMessage, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		writeAPIError(w, 404, "NOT_FOUND", "Operationが見つかりません", "operation")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "DATABASE_ERROR", "Operationを取得できません", "")
		return
	}
	writeJSON(w, 200, map[string]string{"name": "operations/" + o.ID, "kind": strings.ToLower(o.Kind), "state": strings.ToLower(o.State), "phase": o.Phase, "displayMessage": o.DisplayMessage, "errorMessage": o.ErrorMessage, "createdAt": o.CreatedAt, "updatedAt": o.UpdatedAt})
}
func (s *Server) watchOperation(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "データベースを利用できません", "")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	id := operationID(r)
	sub := s.events.Subscribe(id)
	defer sub.Close()
	var state, phase, message string
	err := s.DB.QueryRowContext(r.Context(), `SELECT lower(state),phase,display_message FROM operations WHERE id=?`, id).Scan(&state, &phase, &message)
	if err == sql.ErrNoRows {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "Operationが見つかりません", "operation")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Operationを取得できません", "")
		return
	}
	s.writeEvent(w, f, event{ID: id + "-snapshot", Sequence: 0, Timestamp: time.Now().UTC(), Type: state, Data: map[string]string{"message": message, "phase": phase}})
	if state == "succeeded" || state == "failed" || state == "cancelled" {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-sub.C():
			if !open {
				return
			}
			s.writeEvent(w, f, e)
			if e.Type == "succeeded" || e.Type == "failed" || e.Type == "cancelled" {
				return
			}
		}
	}
}

func (s *Server) writeEvent(w http.ResponseWriter, f http.Flusher, e event) {
	b, _ := json.Marshal(e)
	_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b)
	f.Flush()
}

func (s *Server) logQuery(r *http.Request, watch bool) (LogQuery, error) {
	query := r.URL.Query()
	view := query.Get("view")
	service := query.Get("service")
	cursor := query.Get("cursor")
	if watch {
		cursor = query.Get("after")
	}
	q := LogQuery{ApplicationID: appID(r), View: view, Service: service, Cursor: cursor}
	if !watch {
		if value := query.Get("limit"); value != "" {
			limit, err := strconv.Atoi(value)
			if err != nil {
				return LogQuery{}, errors.New("limitが不正です")
			}
			q.Limit = limit
		}
		for _, item := range []struct {
			value  string
			target **time.Time
		}{{query.Get("startAt"), &q.StartAt}, {query.Get("endAt"), &q.EndAt}} {
			if item.value == "" {
				continue
			}
			parsed, err := time.Parse(time.RFC3339Nano, item.value)
			if err != nil {
				return LogQuery{}, errors.New("時刻がRFC3339ではありません")
			}
			*item.target = &parsed
		}
	}
	return q, nil
}

func (s *Server) ensureLogApplication(ctx context.Context, id string) error {
	if s.DB == nil || s.Logs == nil {
		return errors.New("ログデータベースを利用できません")
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM applications WHERE id=?`, id).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) listLogEntries(w http.ResponseWriter, r *http.Request) {
	if err := s.ensureLogApplication(r.Context(), appID(r)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "アプリが見つかりません", "application")
		} else {
			writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "ログを取得できません", "")
		}
		return
	}
	q, err := s.logQuery(r, false)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), "query")
		return
	}
	page, err := s.Logs.Query(r.Context(), q)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), "query")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) watchLogEntries(w http.ResponseWriter, r *http.Request) {
	if err := s.ensureLogApplication(r.Context(), appID(r)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "アプリが見つかりません", "application")
		} else {
			writeAPIError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "ログを取得できません", "")
		}
		return
	}
	q, err := s.logQuery(r, true)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error(), "query")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	notifications, closeSubscription := s.Logs.Subscribe()
	defer closeSubscription()
	writePage := func() bool {
		for {
			page, err := s.Logs.Query(r.Context(), q)
			if err != nil {
				return false
			}
			for _, entry := range page.Entries {
				body, _ := json.Marshal(entry)
				_, _ = fmt.Fprintf(w, "id: %s\nevent: logEntry\ndata: %s\n\n", entry.Cursor, body)
				f.Flush()
				q.Cursor = entry.Cursor
			}
			if page.NextCursor == "" {
				return true
			}
			q.Cursor = page.NextCursor
		}
	}
	if !writePage() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case _, open := <-notifications:
			if !open || !writePage() {
				return
			}
		case <-time.After(15 * time.Second):
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			f.Flush()
		}
	}
}
