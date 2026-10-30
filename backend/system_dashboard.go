package backend

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

var systemImagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,255}$`)

func SystemDashboardID(installationID string) string {
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://labwebsystem.local/system-apps/"+installationID+"/dashboard"))
	return id.String()
}

func EnsureSystemDashboard(ctx context.Context, db *sql.DB, appsRoot, installationID, version, image, devSource string) (string, error) {
	if !systemImagePattern.MatchString(image) {
		return "", fmt.Errorf("Dashboard image設定が不正です")
	}
	if devSource != "" {
		if !filepath.IsAbs(devSource) {
			return "", fmt.Errorf("Dashboard開発sourceは絶対パスで指定してください")
		}
	}

	id := SystemDashboardID(installationID)
	appRoot := filepath.Join(appsRoot, id)
	source := filepath.Join(appRoot, "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		return "", fmt.Errorf("Dashboard source領域を作成できません: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(appRoot, "runtime"), 0700); err != nil {
		return "", fmt.Errorf("Dashboard runtime領域を作成できません: %w", err)
	}
	compose := "services:\n  web:\n    image: " + image + "\n"
	if devSource != "" {
		compose = "services:\n  web:\n    image: " + image + "\n    working_dir: /app\n    command: [\"sh\", \"-c\", \"test -d node_modules/vite || npm ci --no-audit --no-fund; exec npm run dev -- --host 0.0.0.0 --port 80 --strictPort\"]\n    volumes:\n      - " + strconv.Quote(devSource+":/app") + "\n      - /app/node_modules\n"
	}
	if err := WriteAtomic(filepath.Join(source, "compose.yaml"), []byte(compose), 0600); err != nil {
		return "", fmt.Errorf("Dashboard Compose設定を生成できません: %w", err)
	}
	manifest := "apiVersion: lws/v1\nmetadata:\n  name: LWS Dashboard\n  description: LWSの管理画面\npublic:\n  service: web\n  port: 80\n"
	if err := WriteAtomic(filepath.Join(source, "lws.manifest.yaml"), []byte(manifest), 0600); err != nil {
		return "", fmt.Errorf("Dashboard manifestを生成できません: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO applications(
		id,app_name,repository_url,git_ref,manifest_name,manifest_description,
		desired_state,observed_state,registration_state,installation_id,
		manifest_service,manifest_port,public_service,public_port,compose_file,override_files,is_system,
		created_at,updated_at
	) VALUES(?,?,?,?,?,?, 'RUNNING','UNKNOWN','ACTIVE',?,'web',80,'web',80,'compose.yaml','[]',1,datetime('now'),datetime('now'))
	ON CONFLICT(id) DO UPDATE SET
		app_name=excluded.app_name,repository_url='',git_ref=excluded.git_ref,
		manifest_name=excluded.manifest_name,manifest_description=excluded.manifest_description,
		registration_state='ACTIVE',installation_id=excluded.installation_id,
		manifest_service='web',manifest_port=80,public_service='web',public_port=80,
		compose_file='compose.yaml',override_files='[]',is_system=1,updated_at=datetime('now')`,
		id, "dashboard", "", version, "LWS Dashboard", "LWSの管理画面", installationID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: applications.app_name") {
			return "", fmt.Errorf("dashboardはLWS Dashboard用に予約されています。既存アプリの公開名を変更してください")
		}
		return "", fmt.Errorf("Dashboardをシステムアプリとして登録できません: %w", err)
	}
	return id, nil
}
