package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type RuntimeExecutor struct {
	DB             *sql.DB
	Root           string
	Runner         CommandRunner
	Docker         *DockerResources
	Derived        *DerivedManager
	SecretKey      []byte
	InstallationID string
	DeviceScanner  DeviceScanner
}

func NewRuntimeExecutor(db *sql.DB, root string) *RuntimeExecutor {
	return &RuntimeExecutor{DB: db, Root: root, Runner: OSRunner{}, DeviceScanner: UdevDeviceScanner{Runner: OSRunner{}}}
}

// ReconcileActiveは、SQLiteの正本とDockerの動的なedge network接続を再調整します。
func (e *RuntimeExecutor) ReconcileActive(ctx context.Context) error {
	if e.Docker == nil {
		return nil
	}
	rows, err := e.DB.QueryContext(ctx, `SELECT id FROM applications WHERE registration_state='ACTIVE' AND desired_state='RUNNING' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if err := e.Docker.EnsureNetwork(ctx, id); err != nil {
			return fmt.Errorf("アプリ%sのedge networkを再調整できません: %w", id, err)
		}
		if err := e.Docker.EnsureCaddyConnected(ctx, id); err != nil {
			return fmt.Errorf("アプリ%sのCaddy接続を再調整できません: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func (e *RuntimeExecutor) ReconcileStartup(ctx context.Context) error {
	if err := e.ReconcileActive(ctx); err != nil {
		return err
	}
	return e.syncDerived(ctx)
}

func (e *RuntimeExecutor) Run(ctx context.Context, op Operation) (runErr error) {
	var repo, ref, state, desired, previousName, previousDescription, previousRevision, previousService string
	var previousPort int
	if err := e.DB.QueryRowContext(ctx, `SELECT repository_url,git_ref,registration_state,desired_state,manifest_name,manifest_description,revision,manifest_service,manifest_port FROM applications WHERE id=?`, op.ApplicationID).Scan(&repo, &ref, &state, &desired, &previousName, &previousDescription, &previousRevision, &previousService, &previousPort); err != nil {
		return fmt.Errorf("アプリ情報を取得できません")
	}
	root := filepath.Join(e.Root, op.ApplicationID)
	source := filepath.Join(root, "source")
	runtime := filepath.Join(root, "runtime")
	switch op.Kind {
	case "CONFIGURE":
		reportOperationProgress(ctx, "環境設定を反映しています")
		var desired string
		if err := e.DB.QueryRowContext(ctx, `SELECT desired_state FROM applications WHERE id=?`, op.ApplicationID).Scan(&desired); err != nil {
			return fmt.Errorf("アプリ状態を取得できません")
		}
		if desired != "RUNNING" {
			return nil
		}
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		return e.syncDerived(ctx)
	case "UPDATE":
		var request struct {
			Ref       string `json:"ref"`
			Subdomain string `json:"subdomain"`
		}
		if err := json.Unmarshal([]byte(op.Payload), &request); err != nil {
			return fmt.Errorf("更新内容が不正です")
		}
		var oldSubdomain, oldRef string
		if err := e.DB.QueryRowContext(ctx, `SELECT subdomain,git_ref FROM applications WHERE id=?`, op.ApplicationID).Scan(&oldSubdomain, &oldRef); err != nil {
			return fmt.Errorf("アプリ情報を取得できません")
		}
		newRef, newSubdomain := oldRef, oldSubdomain
		if request.Ref != "" {
			newRef = request.Ref
		}
		if request.Subdomain != "" {
			newSubdomain = request.Subdomain
		}
		if err := ValidateRef(newRef); err != nil {
			return err
		}
		if err := ValidateSubdomain(newSubdomain); err != nil {
			return err
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET subdomain=?,git_ref=?,updated_at=datetime('now') WHERE id=?`, newSubdomain, newRef, op.ApplicationID); err != nil {
			return err
		}
		if err := e.Run(ctx, Operation{ApplicationID: op.ApplicationID, Kind: "SYNC"}); err != nil {
			_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET subdomain=?,git_ref=?,updated_at=datetime('now') WHERE id=?`, oldSubdomain, oldRef, op.ApplicationID)
			return err
		}
		return nil
	case "CREATE", "SYNC":
		reportOperationPhase(ctx, "source_prepare", "GitHubリポジトリの取得を準備しています")
		if err := ValidateRef(ref); err != nil {
			return err
		}
		if err := ValidateRepositoryURL(repo); err != nil {
			return err
		}
		if err := CloneAndValidate(ctx, e.Runner, repo, ref, source); err != nil {
			return err
		}
		reportOperationPhase(ctx, "runtime_prepare", "アプリの実行設定を準備しています")
		sourceSwapActive := true
		metadataUpdated := false
		defer func() {
			if !sourceSwapActive {
				return
			}
			if runErr != nil {
				if metadataUpdated {
					_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET manifest_name=?,manifest_description=?,revision=?,manifest_service=?,manifest_port=?,updated_at=datetime('now') WHERE id=?`, previousName, previousDescription, previousRevision, previousService, previousPort, op.ApplicationID)
				}
				if err := RestoreSourceSwap(source); err != nil {
					runErr = fmt.Errorf("sourceを復元できません: %w（元のエラー: %v）", err, runErr)
				}
				return
			}
			if err := FinalizeSourceSwap(source); err != nil {
				runErr = fmt.Errorf("旧sourceを整理できません: %w", err)
			}
		}()
		if err := os.MkdirAll(runtime, 0700); err != nil {
			return err
		}
		manifestData, err := os.ReadFile(filepath.Join(source, "lws.manifest.yaml"))
		if err != nil {
			return fmt.Errorf("manifestを読み取れません")
		}
		manifest, err := ValidateManifest(manifestData)
		if err != nil {
			return err
		}
		if err := e.GenerateOverrideFromCompose(op.ApplicationID, manifest.Public.Service, filepath.Join(source, "compose.yaml"), filepath.Join(runtime, "lws.override.yaml")); err != nil {
			return err
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET manifest_name=?,manifest_description=?,revision=?,manifest_service=?,manifest_port=?,updated_at=datetime('now') WHERE id=?`, manifest.Metadata.Name, manifest.Metadata.Description, ref, manifest.Public.Service, manifest.Public.Port, op.ApplicationID); err != nil {
			return err
		}
		metadataUpdated = true
		if op.Kind == "CREATE" {
			if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='CONFIGURING',desired_state='STOPPED',observed_state='STOPPED',updated_at=datetime('now') WHERE id=?`, op.ApplicationID); err != nil {
				return err
			}
			reportOperationPhase(ctx, "configuration_required", "設定レイヤーで環境変数とデバイスを確認してください")
			sourceSwapActive = false
			return nil
		}
		if err := e.ensureConfigurationReady(ctx, op.ApplicationID, filepath.Join(source, "compose.yaml")); err != nil {
			return err
		}
		reportOperationPhase(ctx, "compose_up", "Docker containerを作成しています")
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		reportOperationPhase(ctx, "publish", "公開設定を更新しています")
		if err := e.syncDerived(ctx); err != nil {
			return err
		}
		if err := e.markApplicationState(ctx, op.ApplicationID, "RUNNING", "RUNNING", ""); err != nil {
			return err
		}
		sourceSwapActive = false
		return nil
	case "START", "REBUILD":
		if err := e.ensureConfigurationReady(ctx, op.ApplicationID, filepath.Join(source, "compose.yaml")); err != nil {
			return err
		}
		if err := e.GenerateOverrideFromCompose(op.ApplicationID, previousService, filepath.Join(source, "compose.yaml"), filepath.Join(runtime, "lws.override.yaml")); err != nil {
			return err
		}
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		if err := e.syncDerived(ctx); err != nil {
			return err
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='ACTIVE' WHERE id=?`, op.ApplicationID); err != nil {
			return err
		}
		return e.markApplicationState(ctx, op.ApplicationID, "RUNNING", "RUNNING", "")
	case "STOP":
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "stop"); err != nil {
			return err
		}
		return e.markApplicationState(ctx, op.ApplicationID, "STOPPED", "STOPPED", "")
	case "UNREGISTER":
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "down", "--remove-orphans"); err != nil {
			return err
		}
		if e.Docker != nil {
			if err := e.Docker.DisconnectCaddy(ctx, op.ApplicationID); err != nil {
				return err
			}
			if err := e.Docker.RemoveNetwork(ctx, op.ApplicationID, EdgeNetworkName(op.ApplicationID)); err != nil {
				e.restoreActive(ctx, op.ApplicationID, source, runtime, desired)
				return err
			}
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='UNREGISTERED',updated_at=datetime('now') WHERE id=?`, op.ApplicationID); err != nil {
			e.restoreActive(ctx, op.ApplicationID, source, runtime, desired)
			return err
		}
		if err := e.syncDerived(ctx); err != nil {
			_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='ACTIVE',updated_at=datetime('now') WHERE id=?`, op.ApplicationID)
			e.restoreActive(ctx, op.ApplicationID, source, runtime, desired)
			return err
		}
		return nil
	case "REGISTER":
		if state != "UNREGISTERED" {
			return fmt.Errorf("登録解除済みアプリだけ再登録できます")
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='ACTIVE',updated_at=datetime('now') WHERE id=?`, op.ApplicationID); err != nil {
			return err
		}
		if desired == "RUNNING" {
			if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
				_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='UNREGISTERED',updated_at=datetime('now') WHERE id=?`, op.ApplicationID)
				return err
			}
		}
		if err := e.syncDerived(ctx); err != nil {
			_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='UNREGISTERED',updated_at=datetime('now') WHERE id=?`, op.ApplicationID)
			return err
		}
		return nil
	case "PURGE":
		if state == "ACTIVE" {
			reportOperationPhase(ctx, "unregister", "登録を解除してから完全削除します")
			if err := e.Run(ctx, Operation{ApplicationID: op.ApplicationID, Kind: "UNREGISTER"}); err != nil {
				return err
			}
			state = "UNREGISTERED"
		}
		if state != "UNREGISTERED" {
			return fmt.Errorf("登録解除済みアプリだけ完全削除できます")
		}
		reportOperationPhase(ctx, "volume_cleanup", "アプリのDocker volumeを削除しています")
		if e.Docker != nil {
			if err := e.Docker.RemoveOwnedVolumes(ctx, op.ApplicationID); err != nil {
				return err
			}
		}
		reportOperationPhase(ctx, "filesystem_cleanup", "アプリの作業領域を削除しています")
		if err := os.RemoveAll(root); err != nil {
			return err
		}
		reportOperationPhase(ctx, "database_cleanup", "アプリの保存データを削除しました")
		return nil
	default:
		return fmt.Errorf("未対応のOperationです: %s", op.Kind)
	}
}

func (e *RuntimeExecutor) ensureConfigurationReady(ctx context.Context, id, compose string) error {
	if _, err := RefreshLWSDevices(ctx, e.DB, e.DeviceScanner); err != nil {
		return fmt.Errorf("LWSデバイスの接続状態を確認できません: %w", err)
	}
	data, err := os.ReadFile(compose)
	if err != nil {
		return err
	}
	attachments, err := ComposeDeviceAttachments(data)
	if err != nil {
		return err
	}
	variables, err := ExtractComposeVariables(data)
	if err != nil {
		return err
	}
	for _, v := range variables {
		if v.Required {
			var count int
			if err := e.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM application_variables WHERE application_id=? AND name=?`, id, v.Name).Scan(&count); err != nil || count == 0 {
				return fmt.Errorf("必須環境変数%sを設定してください", v.Name)
			}
		}
	}
	for _, a := range attachments {
		var path, status string
		err := e.DB.QueryRowContext(ctx, `SELECT d.current_path,d.status FROM application_device_bindings b JOIN lws_devices d ON d.id=b.device_id WHERE b.application_id=? AND b.service=? AND b.target_path=?`, id, a.Service, a.Target).Scan(&path, &status)
		if err != nil || status != "CONNECTED" || !strings.HasPrefix(path, "/dev/") {
			return fmt.Errorf("%s のデバイス%sをLWSデバイスプールから割り当ててください", a.Service, a.Target)
		}
	}
	return nil
}

func (e *RuntimeExecutor) restoreActive(ctx context.Context, id, source, runtime, desired string) {
	if e.Docker != nil {
		if e.Docker.EnsureNetwork(ctx, id) == nil && desired == "RUNNING" {
			_ = e.Docker.EnsureCaddyConnected(ctx, id)
			_ = e.reconcile(ctx, id, source, runtime, "up", "-d")
		}
	}
	_ = e.syncDerived(ctx)
}
func (e *RuntimeExecutor) syncDerived(ctx context.Context) error {
	if e.Derived == nil {
		return nil
	}
	if err := e.Derived.Validate(); err != nil {
		return err
	}
	return e.Derived.Sync(ctx)
}

func (e *RuntimeExecutor) markApplicationState(ctx context.Context, id, desired, observed, latestError string) error {
	_, err := e.DB.ExecContext(ctx, `UPDATE applications SET desired_state=?,observed_state=?,latest_error=?,updated_at=datetime('now') WHERE id=?`, desired, observed, latestError, id)
	return err
}
func (e *RuntimeExecutor) reconcile(ctx context.Context, id, source, runtime string, action ...string) (runErr error) {
	compose := filepath.Join(source, "compose.yaml")
	override := filepath.Join(runtime, "lws.override.yaml")
	env := filepath.Join(runtime, "app.env")
	if _, err := os.Stat(compose); err != nil {
		// sourceが失われても、停止対象のcontainerがないことを所有確認できれば
		// 登録解除・完全削除を冪等に完了できる。containerが残っている場合は
		// Compose定義なしに削除せず、従来どおり失敗させる。
		if len(action) > 0 && action[0] == "down" && e.Docker != nil {
			if ownershipErr := e.Docker.VerifyProjectOwnership(ctx, id); ownershipErr != nil {
				return ownershipErr
			}
			reportOperationPhase(ctx, "compose_execute", "Compose定義がないため実行環境を確認しました")
			return nil
		}
		return fmt.Errorf("compose.yamlが見つかりません")
	}
	if len(action) > 0 && action[0] == "up" {
		reportOperationPhase(ctx, "environment_prepare", "環境変数を準備しています")
		manifestData, err := os.ReadFile(filepath.Join(source, "lws.manifest.yaml"))
		if err != nil {
			return fmt.Errorf("manifestが見つかりません")
		}
		manifest, err := ValidateManifest(manifestData)
		if err != nil {
			return err
		}
		if err := e.prepareEnvironment(ctx, compose, runtime); err != nil {
			return err
		}
		reportOperationPhase(ctx, "compose_validate", "Compose設定を検証しています")
		if err := e.validateEffectiveCompose(ctx, id, compose, "", runtime, manifest.Public.Service, manifest.Public.Port); err != nil {
			return err
		}
	}
	if _, err := os.Stat(override); os.IsNotExist(err) {
		if err := WriteAtomic(override, []byte("services: {}\n"), 0600); err != nil {
			return err
		}
	}
	if len(action) > 0 && action[0] == "up" {
		manifestData, err := os.ReadFile(filepath.Join(source, "lws.manifest.yaml"))
		if err != nil {
			return fmt.Errorf("manifestが見つかりません")
		}
		manifest, err := ValidateManifest(manifestData)
		if err != nil {
			return err
		}
		reportOperationPhase(ctx, "compose_validate", "公開用のCompose設定を検証しています")
		if err := e.validateEffectiveCompose(ctx, id, compose, override, runtime, manifest.Public.Service, manifest.Public.Port); err != nil {
			return err
		}
	}
	if _, err := os.Stat(env); os.IsNotExist(err) {
		if err := WriteAtomic(env, nil, 0600); err != nil {
			return err
		}
	}
	if len(action) > 0 && e.Docker != nil {
		if err := e.Docker.VerifyProjectOwnership(ctx, id); err != nil {
			return err
		}
		if action[0] == "up" {
			reportOperationPhase(ctx, "network_prepare", "Docker networkを準備しています")
			if err := e.Docker.EnsureNetwork(ctx, id); err != nil {
				return err
			}
			reportOperationPhase(ctx, "proxy_connect", "公開ネットワークへ接続しています")
			if err := e.Docker.EnsureCaddyConnected(ctx, id); err != nil {
				return err
			}
			caddyConnected := true
			defer func() {
				if runErr == nil || !caddyConnected {
					return
				}
				_ = e.Docker.DisconnectCaddy(ctx, id)
			}()
		}
	}
	args := []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose, "-f", override}
	args = append(args, action...)
	if len(action) > 0 && action[0] == "up" {
		reportOperationPhase(ctx, "container_create", "Docker containerを作成しています")
	} else {
		reportOperationPhase(ctx, "compose_execute", "Docker Composeを実行しています")
	}
	if _, err := runLogged(ctx, e.Runner, "Compose実行", "docker", args...); err != nil {
		return fmt.Errorf("Docker Compose操作に失敗しました")
	}
	return nil
}

func (e *RuntimeExecutor) prepareEnvironment(ctx context.Context, compose, runtime string) error {
	if e.DB == nil {
		return fmt.Errorf("データベースを利用できません")
	}
	if err := os.MkdirAll(runtime, 0700); err != nil {
		return err
	}
	data, err := os.ReadFile(compose)
	if err != nil {
		return fmt.Errorf("Composeを読み取れません")
	}
	references, err := ExtractComposeVariables(data)
	if err != nil {
		return err
	}
	values := map[string]string{}
	rows, err := e.DB.QueryContext(ctx, `SELECT name,value,is_secret FROM application_variables WHERE application_id=?`, filepath.Base(filepath.Dir(runtime)))
	if err != nil {
		return fmt.Errorf("環境変数を取得できません")
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var stored []byte
		var secret bool
		if err := rows.Scan(&name, &stored, &secret); err != nil {
			return fmt.Errorf("環境変数を取得できません")
		}
		if secret {
			if len(e.SecretKey) == 0 {
				return fmt.Errorf("secret鍵を利用できません")
			}
			stored, err = Decrypt(e.SecretKey, stored)
			if err != nil {
				return fmt.Errorf("secretを復号できません")
			}
		}
		values[name] = string(stored)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("環境変数を取得できません")
	}
	if err := ValidateComposeVariableValues(references, values); err != nil {
		return err
	}
	var output strings.Builder
	for _, reference := range references {
		value, exists := values[reference.Name]
		if !exists {
			continue
		}
		if strings.ContainsAny(value, "\r\n") {
			return NewValidationError(reference.Name, "環境変数に改行は指定できません", "INVALID_ENVIRONMENT_VARIABLE")
		}
		output.WriteString(reference.Name)
		output.WriteByte('=')
		output.WriteString(strconv.Quote(value))
		output.WriteByte('\n')
	}
	return WriteAtomic(filepath.Join(runtime, "app.env"), []byte(output.String()), 0600)
}

func (e *RuntimeExecutor) validateEffectiveCompose(ctx context.Context, id, compose, override, runtime, service string, port int) error {
	env := filepath.Join(runtime, "app.env")
	if err := os.MkdirAll(runtime, 0700); err != nil {
		return err
	}
	if _, err := os.Stat(env); os.IsNotExist(err) {
		if err := WriteAtomic(env, nil, 0600); err != nil {
			return err
		}
	}
	args := []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose, "config", "--format", "json"}
	if override != "" {
		args = []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose, "-f", override, "config", "--format", "json"}
	}
	out, err := runLoggedJSON(ctx, e.Runner, "Compose検証", "docker", args...)
	if err != nil {
		return fmt.Errorf("Compose設定の検証に失敗しました")
	}
	if override == "" {
		return ValidateEffectiveCompose(out, service, port)
	}
	return ValidateEffectiveComposeWithOwnedNetwork(out, service, port, EdgeNetworkName(id))
}
func (e *RuntimeExecutor) GenerateOverride(id, service, path string) error {
	return e.GenerateOverrideWithVolumes(id, service, nil, path)
}

func (e *RuntimeExecutor) GenerateOverrideFromCompose(id, service, compose string, path string) error {
	data, err := os.ReadFile(compose)
	if err != nil {
		return fmt.Errorf("Composeを読み取れません")
	}
	volumes, err := NamedVolumeNames(data)
	if err != nil {
		return err
	}
	services, err := ComposeServiceNames(data)
	if err != nil {
		return err
	}
	if err := e.GenerateOverrideWithServicesAndVolumes(id, service, services, volumes, path); err != nil {
		return err
	}
	attachments, err := ComposeDeviceAttachments(data)
	if err != nil {
		return err
	}
	if len(attachments) == 0 {
		return nil
	}
	overrideData, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var model map[string]any
	if err := json.Unmarshal(overrideData, &model); err != nil {
		return err
	}
	serviceModels := model["services"].(map[string]any)
	for _, a := range attachments {
		var actual string
		err := e.DB.QueryRow(`SELECT d.current_path FROM application_device_bindings b JOIN lws_devices d ON d.id=b.device_id WHERE b.application_id=? AND b.service=? AND b.target_path=?`, id, a.Service, a.Target).Scan(&actual)
		if err == nil && strings.HasPrefix(actual, "/dev/") {
			sm := serviceModels[a.Service].(map[string]any)
			sm["devices"] = appendDevice(sm["devices"], actual+":"+a.Target+":"+a.Permissions)
		}
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		return err
	}
	return WriteAtomic(path, encoded, 0600)
}

func appendDevice(value any, entry string) []string {
	result := []string{}
	if items, ok := value.([]string); ok {
		result = append(result, items...)
	}
	return append(result, entry)
}

func (e *RuntimeExecutor) GenerateOverrideWithVolumes(id, service string, volumes []string, path string) error {
	return e.GenerateOverrideWithServicesAndVolumes(id, service, []string{service}, volumes, path)
}

func (e *RuntimeExecutor) GenerateOverrideWithServicesAndVolumes(id, publicService string, services, volumes []string, path string) error {
	labels := map[string]string{
		"com.labwebsystem.owner":           "lws",
		"com.labwebsystem.installation-id": e.InstallationID,
		"com.labwebsystem.app-id":          id,
	}
	volumeModels := map[string]any{}
	for _, name := range volumes {
		volumeModels[name] = map[string]any{"labels": labels}
	}
	serviceModels := map[string]any{}
	for _, name := range services {
		serviceModels[name] = map[string]any{"labels": labels}
	}
	public, ok := serviceModels[publicService].(map[string]any)
	if !ok {
		return fmt.Errorf("公開serviceがComposeにありません")
	}
	public["networks"] = map[string]any{"lws-edge": map[string]any{"aliases": []string{"lws-" + id}}}
	model := map[string]any{
		"services": serviceModels,
		"networks": map[string]any{"lws-edge": map[string]any{"external": true, "name": EdgeNetworkName(id)}},
	}
	if len(volumeModels) > 0 {
		model["volumes"] = volumeModels
	}
	data, err := json.Marshal(model)
	if err != nil {
		return err
	}
	return WriteAtomic(path, data, 0600)
}
