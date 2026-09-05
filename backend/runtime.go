package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
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
	var repo, ref, state, desired, previousName, previousDescription, previousRevision, manifestService, composeFile, overrideFilesJSON, previousService string
	var previousPort, manifestPort int
	if err := e.DB.QueryRowContext(ctx, `SELECT repository_url,git_ref,registration_state,desired_state,manifest_name,manifest_description,revision,manifest_service,manifest_port,public_service,public_port,compose_file,override_files FROM applications WHERE id=?`, op.ApplicationID).Scan(&repo, &ref, &state, &desired, &previousName, &previousDescription, &previousRevision, &manifestService, &manifestPort, &previousService, &previousPort, &composeFile, &overrideFilesJSON); err != nil {
		return fmt.Errorf("アプリ情報を取得できません")
	}
	if previousService == "" {
		previousService, previousPort = manifestService, manifestPort
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
		var configuredService string
		var configuredPort int
		if err := e.DB.QueryRowContext(ctx, `SELECT public_service,public_port FROM applications WHERE id=?`, op.ApplicationID).Scan(&configuredService, &configuredPort); err != nil || configuredService == "" || configuredPort < 1 {
			configuredService, configuredPort = previousService, previousPort
		}
		var configureOverrides []string
		_ = json.Unmarshal([]byte(overrideFilesJSON), &configureOverrides)
		overridePaths := make([]string, 0, len(configureOverrides))
		for _, file := range configureOverrides {
			overridePaths = append(overridePaths, filepath.Join(source, file))
		}
		if err := e.GenerateOverrideFromComposes(op.ApplicationID, configuredService, filepath.Join(source, composeFile), overridePaths, filepath.Join(runtime, "lws.override.yaml")); err != nil {
			return err
		}
		if err := e.GenerateEffectiveCompose(ctx, op.ApplicationID, filepath.Join(source, composeFile), overridePaths, filepath.Join(runtime, "lws.override.yaml"), filepath.Join(runtime, "lws.effective.yaml")); err != nil {
			return err
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
		requestedCompose := composeFile
		var requestedOverrides []string
		_ = json.Unmarshal([]byte(overrideFilesJSON), &requestedOverrides)
		if op.Payload != "" {
			var p struct {
				ComposeFile   string   `json:"composeFile"`
				OverrideFiles []string `json:"overrideFiles"`
			}
			_ = json.Unmarshal([]byte(op.Payload), &p)
			if p.ComposeFile != "" {
				requestedCompose = p.ComposeFile
			}
			if p.OverrideFiles != nil {
				requestedOverrides = p.OverrideFiles
			}
		}
		selectedCompose, err := CloneAndValidateWithOverrides(ctx, e.Runner, repo, ref, requestedCompose, requestedOverrides, source)
		if err != nil {
			return err
		}
		composeFile = selectedCompose
		reportOperationPhase(ctx, "runtime_prepare", "アプリの実行設定を準備しています")
		sourceSwapActive := true
		metadataUpdated := false
		defer func() {
			if !sourceSwapActive {
				return
			}
			if runErr != nil {
				if metadataUpdated {
					_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET manifest_name=?,manifest_description=?,revision=?,manifest_service=?,manifest_port=?,updated_at=datetime('now') WHERE id=?`, previousName, previousDescription, previousRevision, manifestService, manifestPort, op.ApplicationID)
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
		composePath := filepath.Join(source, composeFile)
		overridePaths := make([]string, 0, len(requestedOverrides))
		for _, file := range requestedOverrides {
			overridePaths = append(overridePaths, filepath.Join(source, file))
		}
		if err := e.GenerateOverrideFromComposes(op.ApplicationID, manifest.Public.Service, composePath, overridePaths, filepath.Join(runtime, "lws.override.yaml")); err != nil {
			return err
		}
		if err := e.GenerateEffectiveCompose(ctx, op.ApplicationID, composePath, overridePaths, filepath.Join(runtime, "lws.override.yaml"), filepath.Join(runtime, "lws.effective.yaml")); err != nil {
			return err
		}
		overridesJSON, _ := json.Marshal(requestedOverrides)
		if previousService == "" || op.Kind == "CREATE" {
			previousService, previousPort = manifest.Public.Service, manifest.Public.Port
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET manifest_name=?,manifest_description=?,revision=?,manifest_service=?,manifest_port=?,public_service=?,public_port=?,compose_file=?,override_files=?,updated_at=datetime('now') WHERE id=?`, manifest.Metadata.Name, manifest.Metadata.Description, ref, manifest.Public.Service, manifest.Public.Port, previousService, previousPort, composeFile, string(overridesJSON), op.ApplicationID); err != nil {
			return err
		}
		metadataUpdated = true
		// 初回登録と、停止中アプリの再同期は、sourceを再取得した時点で
		// 設定レイヤーへ戻す。device binding未設定のまま起動はしない。
		if op.Kind == "CREATE" || (op.Kind == "SYNC" && desired != "RUNNING") {
			if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='CONFIGURING',desired_state='STOPPED',observed_state='STOPPED',latest_error='',updated_at=datetime('now') WHERE id=?`, op.ApplicationID); err != nil {
				return err
			}
			reportOperationPhase(ctx, "configuration_required", "設定レイヤーで環境変数とデバイスを確認してください")
			sourceSwapActive = false
			return nil
		}
		if err := e.ensureConfigurationReady(ctx, op.ApplicationID, filepath.Join(source, composeFile)); err != nil {
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
		if err := e.ensureConfigurationReady(ctx, op.ApplicationID, filepath.Join(source, composeFile)); err != nil {
			return err
		}
		var startOverrides []string
		_ = json.Unmarshal([]byte(overrideFilesJSON), &startOverrides)
		overridePaths := make([]string, 0, len(startOverrides))
		for _, file := range startOverrides {
			overridePaths = append(overridePaths, filepath.Join(source, file))
		}
		if err := e.GenerateOverrideFromComposes(op.ApplicationID, previousService, filepath.Join(source, composeFile), overridePaths, filepath.Join(runtime, "lws.override.yaml")); err != nil {
			return err
		}
		if err := e.GenerateEffectiveCompose(ctx, op.ApplicationID, filepath.Join(source, composeFile), overridePaths, filepath.Join(runtime, "lws.override.yaml"), filepath.Join(runtime, "lws.effective.yaml")); err != nil {
			return err
		}
		if op.Kind == "REBUILD" {
			reportOperationPhase(ctx, "image_pull", "Docker imageを取得しています")
			if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "pull"); err != nil {
				return err
			}
			reportOperationPhase(ctx, "image_build", "Docker imageをビルドしています")
			if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "build", "--pull"); err != nil {
				return err
			}
		}
		upArgs := []string{"up", "-d"}
		if op.Kind == "REBUILD" {
			upArgs = append(upArgs, "--force-recreate", "--remove-orphans")
		}
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, upArgs...); err != nil {
			return err
		}
		// CONFIGURINGからの初回起動では、公開設定の再生成前にACTIVEへ遷移
		// させる必要がある。DerivedManagerはACTIVEアプリだけを公開する。
		wasActive := state == "ACTIVE"
		if !wasActive {
			if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='ACTIVE',updated_at=datetime('now') WHERE id=?`, op.ApplicationID); err != nil {
				return err
			}
		}
		if err := e.syncDerived(ctx); err != nil {
			if !wasActive {
				_, _ = e.DB.ExecContext(ctx, `UPDATE applications SET registration_state=?,updated_at=datetime('now') WHERE id=?`, state, op.ApplicationID)
			}
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
		if state == "ACTIVE" || state == "CONFIGURING" {
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
	sources, err := e.selectedComposeSources(id, compose)
	if err != nil {
		return err
	}
	variables, err := MergeComposeVariables(sources)
	if err != nil {
		return err
	}
	attachments := []DeviceAttachment{}
	for _, source := range sources {
		found, attachmentErr := ComposeDeviceAttachments(source.Data)
		if attachmentErr != nil {
			return attachmentErr
		}
		attachments = append(attachments, found...)
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
	composeFile := "compose.yaml"
	if err := e.DB.QueryRow(`SELECT compose_file FROM applications WHERE id=?`, id).Scan(&composeFile); err != nil || !containsComposeFile(composeFile) {
		composeFile = "compose.yaml"
	}
	compose := filepath.Join(source, composeFile)
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
		return fmt.Errorf("%sが見つかりません", composeFile)
	}
	if len(action) > 0 && action[0] == "up" {
		reportOperationPhase(ctx, "environment_prepare", "環境変数を準備しています")
		if err := e.prepareEnvironment(ctx, compose, runtime); err != nil {
			return err
		}
		// 公開用の生成overrideを含めた実効Composeを、下の検証で一度だけ検査する。
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
		publicService, publicPort := manifest.Public.Service, manifest.Public.Port
		var configuredService string
		var configuredPort int
		if err := e.DB.QueryRowContext(ctx, `SELECT public_service,public_port FROM applications WHERE id=?`, id).Scan(&configuredService, &configuredPort); err == nil && configuredService != "" && configuredPort > 0 {
			publicService, publicPort = configuredService, configuredPort
		}
		reportOperationPhase(ctx, "compose_validate", "公開用のCompose設定を検証しています")
		if err := e.validateEffectiveCompose(ctx, id, compose, override, runtime, publicService, publicPort); err != nil {
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
	args := []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose}
	if files, err := readOverrideFiles(e.DB, id); err == nil {
		for _, file := range files {
			args = append(args, "-f", filepath.Join(source, file))
		}
	}
	args = append(args, "-f", override)
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

func readOverrideFiles(db *sql.DB, id string) ([]string, error) {
	var encoded string
	if err := db.QueryRow(`SELECT override_files FROM applications WHERE id=?`, id).Scan(&encoded); err != nil {
		return nil, err
	}
	var files []string
	if encoded == "" {
		return files, nil
	}
	if err := json.Unmarshal([]byte(encoded), &files); err != nil {
		return nil, err
	}
	for _, file := range files {
		if !validComposeFilename(file) {
			return nil, fmt.Errorf("override Composeファイル名が不正です")
		}
	}
	return files, nil
}

func (e *RuntimeExecutor) prepareEnvironment(ctx context.Context, compose, runtime string) error {
	if e.DB == nil {
		return fmt.Errorf("データベースを利用できません")
	}
	if err := os.MkdirAll(runtime, 0700); err != nil {
		return err
	}
	id := filepath.Base(filepath.Dir(runtime))
	sources, err := e.selectedComposeSources(id, compose)
	if err != nil {
		return fmt.Errorf("Composeを読み取れません")
	}
	references, err := MergeComposeVariables(sources)
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

func (e *RuntimeExecutor) selectedComposeSources(id, compose string) ([]ComposeSource, error) {
	var encoded string
	if err := e.DB.QueryRow(`SELECT override_files FROM applications WHERE id=?`, id).Scan(&encoded); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	var overrides []string
	if encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &overrides); err != nil {
			return nil, err
		}
	}
	return ReadComposeSources(filepath.Dir(compose), filepath.Base(compose), overrides)
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
		args = []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose}
		if files, err := readOverrideFiles(e.DB, id); err == nil {
			for _, file := range files {
				args = append(args, "-f", filepath.Join(filepath.Dir(runtime), "source", file))
			}
		}
		args = append(args, "-f", override, "config", "--format", "json")
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

// GenerateEffectiveComposeは、利用者が選択したCompose群とLWS overrideを
// Docker Compose自身にマージさせた、確認用の完全なComposeを生成する。
func (e *RuntimeExecutor) GenerateEffectiveCompose(ctx context.Context, id, compose string, overrides []string, override, path string) error {
	env := filepath.Join(filepath.Dir(path), "app.env")
	if _, err := os.Stat(env); os.IsNotExist(err) {
		if err := WriteAtomic(env, nil, 0600); err != nil {
			return err
		}
	}
	args := []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose}
	for _, file := range overrides {
		args = append(args, "-f", file)
	}
	args = append(args, "-f", override, "config", "--no-interpolate", "--format", "yaml")
	data, err := e.Runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("マージ後のComposeを生成できません")
	}
	if len(data) == 0 {
		return fmt.Errorf("マージ後のComposeが空です")
	}
	return WriteAtomic(path, data, 0600)
}
func (e *RuntimeExecutor) GenerateOverride(id, service, path string) error {
	return e.GenerateOverrideWithVolumes(id, service, nil, path)
}

func (e *RuntimeExecutor) GenerateOverrideFromCompose(id, service, compose string, path string) error {
	return e.GenerateOverrideFromComposes(id, service, compose, nil, path)
}

func (e *RuntimeExecutor) GenerateOverrideFromComposes(id, service, compose string, overrides []string, path string) error {
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
	serviceSet := map[string]bool{}
	for _, name := range services {
		serviceSet[name] = true
	}
	for _, file := range overrides {
		contents, readErr := os.ReadFile(file)
		if readErr != nil {
			return fmt.Errorf("override Composeを読み取れません")
		}
		names, nameErr := ComposeServiceNames(contents)
		if nameErr == nil {
			for _, name := range names {
				if !serviceSet[name] {
					services = append(services, name)
					serviceSet[name] = true
				}
			}
		}
		named, namedErr := NamedVolumeNames(contents)
		if namedErr == nil {
			volumes = append(volumes, named...)
		}
	}
	sort.Strings(services)
	sort.Strings(volumes)
	if err := e.GenerateOverrideWithServicesAndVolumes(id, service, services, volumes, path); err != nil {
		return err
	}
	composeSources := []ComposeSource{{Path: compose, Data: data}}
	for _, file := range overrides {
		contents, readErr := os.ReadFile(file)
		if readErr != nil {
			return fmt.Errorf("override Composeを読み取れません")
		}
		composeSources = append(composeSources, ComposeSource{Path: file, Data: contents})
	}
	interfaces, err := ComposeWebInterfaces(composeSources)
	if err != nil {
		return err
	}
	attachments := []DeviceAttachment{}
	for _, source := range composeSources {
		found, attachmentErr := ComposeDeviceAttachments(source.Data)
		if attachmentErr != nil {
			return attachmentErr
		}
		attachments = append(attachments, found...)
	}
	// 公開serviceがoverride側にのみ定義される場合でも、元のdefault
	// networkを維持して内部serviceへ接続できるようにする。
	if err := e.preservePublicServiceNetworks(id, service, composeSources, path); err != nil {
		return err
	}
	if len(attachments) > 0 {
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
		if err := WriteAtomic(path, encoded, 0600); err != nil {
			return err
		}
	}
	return e.addHostPortsToOverride(interfaces, path)
}

func (e *RuntimeExecutor) preservePublicServiceNetworks(id, service string, sources []ComposeSource, path string) error {
	networks, err := ComposeServiceNetworkNamesFromSources(sources, service)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var model map[string]any
	if err := json.Unmarshal(data, &model); err != nil {
		return err
	}
	serviceModels, ok := model["services"].(map[string]any)
	if !ok {
		return fmt.Errorf("生成overrideのservicesが不正です")
	}
	publicModel, ok := serviceModels[service].(map[string]any)
	if !ok {
		return fmt.Errorf("公開serviceがComposeにありません")
	}
	publicNetworks := map[string]any{}
	for _, network := range networks {
		publicNetworks[network] = map[string]any{}
	}
	publicNetworks["lws-edge"] = map[string]any{"aliases": []string{"lws-" + id}}
	publicModel["networks"] = publicNetworks
	encoded, err := json.Marshal(model)
	if err != nil {
		return err
	}
	return WriteAtomic(path, encoded, 0600)
}

func (e *RuntimeExecutor) addHostPortsToOverride(interfaces []WebInterface, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var model map[string]any
	if err := json.Unmarshal(data, &model); err != nil {
		return err
	}
	services, ok := model["services"].(map[string]any)
	if !ok {
		return fmt.Errorf("生成overrideのservicesが不正です")
	}
	for _, service := range services {
		if definition, ok := service.(map[string]any); ok {
			// Composeのportsはhost公開を意味するため、!resetで確実に無効化する。
			definition["ports"] = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!reset"}
		}
	}
	for _, iface := range interfaces {
		definition, ok := services[iface.Service].(map[string]any)
		if !ok {
			continue
		}
		current, _ := definition["expose"].([]any)
		port := strconv.Itoa(iface.Port)
		found := false
		for _, item := range current {
			if fmt.Sprint(item) == port {
				found = true
				break
			}
		}
		if !found {
			definition["expose"] = append(current, port)
		}
	}
	encoded, err := yaml.Marshal(model)
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
