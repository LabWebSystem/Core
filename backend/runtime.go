package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type RuntimeExecutor struct {
	DB      *sql.DB
	Root    string
	Runner  CommandRunner
	Docker  *DockerResources
	Derived *DerivedManager
}

func NewRuntimeExecutor(db *sql.DB, root string) *RuntimeExecutor {
	return &RuntimeExecutor{DB: db, Root: root, Runner: OSRunner{}}
}
func (e *RuntimeExecutor) Run(ctx context.Context, op Operation) error {
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
		if err := e.GenerateOverride(op.ApplicationID, manifest.Public.Service, filepath.Join(runtime, "lws.override.yaml")); err != nil {
			return err
		}
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		return e.syncDerived(ctx)
	case "START", "REBUILD":
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "up", "-d"); err != nil {
			return err
		}
		return e.syncDerived(ctx)
	case "STOP":
		return e.reconcile(ctx, op.ApplicationID, source, runtime, "stop")
	case "UNREGISTER":
		if err := e.reconcile(ctx, op.ApplicationID, source, runtime, "down", "--remove-orphans"); err != nil {
			return err
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
func (e *RuntimeExecutor) reconcile(ctx context.Context, id, source, runtime string, action ...string) error {
	compose := filepath.Join(source, "compose.yaml")
	override := filepath.Join(runtime, "lws.override.yaml")
	env := filepath.Join(runtime, "app.env")
	if _, err := os.Stat(compose); err != nil {
		return fmt.Errorf("compose.yamlが見つかりません")
	}
	if _, err := os.Stat(override); os.IsNotExist(err) {
		if err := WriteAtomic(override, []byte("services: {}\n"), 0600); err != nil {
			return err
		}
	}
	if _, err := os.Stat(env); os.IsNotExist(err) {
		if err := WriteAtomic(env, nil, 0600); err != nil {
			return err
		}
	}
	if len(action) > 0 && action[0] == "up" && e.Docker != nil {
		if err := e.Docker.EnsureNetwork(ctx, id); err != nil {
			return err
		}
	}
	args := []string{"compose", "--project-name", ProjectName(id), "--env-file", env, "-f", compose, "-f", override}
	args = append(args, action...)
	if _, err := e.Runner.Run(ctx, "docker", args...); err != nil {
		return fmt.Errorf("Docker Compose操作に失敗しました")
	}
	return nil
}
func (e *RuntimeExecutor) GenerateOverride(id, service, path string) error {
	model := map[string]any{
		"services": map[string]any{service: map[string]any{"networks": map[string]any{"lws-edge": map[string]any{"aliases": []string{"lws-" + id}}}}},
		"networks": map[string]any{"lws-edge": map[string]any{"external": true, "name": EdgeNetworkName(id)}},
	}
	data, err := json.Marshal(model)
	if err != nil {
		return err
	}
	return WriteAtomic(path, data, 0600)
}
