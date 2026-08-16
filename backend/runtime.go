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
}

func NewRuntimeExecutor(db *sql.DB, root string) *RuntimeExecutor {
	return &RuntimeExecutor{DB: db, Root: root, Runner: OSRunner{}}
}
func (e *RuntimeExecutor) Run(ctx context.Context, op Operation) (runErr error) {
	var repo, ref, state string
	if err := e.DB.QueryRowContext(ctx, `SELECT repository_url,git_ref,registration_state FROM applications WHERE id=?`, op.ApplicationID).Scan(&repo, &ref, &state); err != nil {
		return fmt.Errorf("アプリ情報を取得できません")
	}
	root := filepath.Join(e.Root, op.ApplicationID)
	source := filepath.Join(root, "source")
	runtime := filepath.Join(root, "runtime")
	switch op.Kind {
	case "CREATE", "SYNC":
		if err := ValidateRef(ref); err != nil {
			return err
		}
		if err := ValidateRepositoryURL(repo); err != nil {
			return err
		}
		if err := CloneAndValidate(ctx, e.Runner, repo, ref, source); err != nil {
			return err
		}
		sourceSwapActive := true
		defer func() {
			if !sourceSwapActive {
				return
			}
			if runErr != nil {
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
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET manifest_name=?,manifest_description=?,revision=?,updated_at=datetime('now') WHERE id=?`, manifest.Metadata.Name, manifest.Metadata.Description, ref, op.ApplicationID); err != nil {
			return err
		}
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		if err := e.syncDerived(ctx); err != nil {
			return err
		}
		if err := e.markApplicationState(ctx, op.ApplicationID, "RUNNING", "RUNNING", ""); err != nil {
			return err
		}
		sourceSwapActive = false
		return nil
	case "START", "REBUILD":
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		if err := e.syncDerived(ctx); err != nil {
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
				return err
			}
		}
		if _, err := e.DB.ExecContext(ctx, `UPDATE applications SET registration_state='UNREGISTERED',updated_at=datetime('now') WHERE id=?`, op.ApplicationID); err != nil {
			return err
		}
		if err := os.RemoveAll(root); err != nil {
			return err
		}
		return e.syncDerived(ctx)
	case "PURGE":
		if state != "UNREGISTERED" {
			return fmt.Errorf("登録解除済みアプリだけ完全削除できます")
		}
		if e.Docker != nil {
			if err := e.Docker.RemoveOwnedVolumes(ctx, op.ApplicationID); err != nil {
				return err
			}
		}
		_, err := e.DB.ExecContext(ctx, `DELETE FROM applications WHERE id=?`, op.ApplicationID)
		return err
	default:
		return fmt.Errorf("未対応のOperationです: %s", op.Kind)
	}
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
func (e *RuntimeExecutor) reconcile(ctx context.Context, id, source, runtime string, action ...string) error {
	compose := filepath.Join(source, "compose.yaml")
	override := filepath.Join(runtime, "lws.override.yaml")
	env := filepath.Join(runtime, "app.env")
	if _, err := os.Stat(compose); err != nil {
		return fmt.Errorf("compose.yamlが見つかりません")
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
		if err := e.prepareEnvironment(ctx, compose, runtime); err != nil {
			return err
		}
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
			if err := e.Docker.EnsureNetwork(ctx, id); err != nil {
				return err
			}
			if err := e.Docker.EnsureCaddyConnected(ctx, id); err != nil {
				return err
			}
		}
	}
	args := []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose, "-f", override}
	args = append(args, action...)
	if _, err := e.Runner.Run(ctx, "docker", args...); err != nil {
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
	out, err := e.Runner.Run(ctx, "docker", args...)
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
	return e.GenerateOverrideWithVolumes(id, service, volumes, path)
}

func (e *RuntimeExecutor) GenerateOverrideWithVolumes(id, service string, volumes []string, path string) error {
	labels := map[string]string{
		"com.labwebsystem.owner":           "lws",
		"com.labwebsystem.installation-id": e.InstallationID,
		"com.labwebsystem.app-id":          id,
	}
	volumeModels := map[string]any{}
	for _, name := range volumes {
		volumeModels[name] = map[string]any{"labels": labels}
	}
	model := map[string]any{
		"services": map[string]any{service: map[string]any{
			"labels": map[string]string{
				"com.labwebsystem.owner":           "lws",
				"com.labwebsystem.installation-id": e.InstallationID,
				"com.labwebsystem.app-id":          id,
			},
			"networks": map[string]any{"lws-edge": map[string]any{"aliases": []string{"lws-" + id}}},
		}},
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
