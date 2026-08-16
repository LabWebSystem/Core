package backend

import (
	"fmt"
	"strings"
)

type ResourceLabels map[string]string

func VerifyOwnership(labels ResourceLabels, installation, app string) error {
	if labels["com.labwebsystem.owner"] != "lws" || labels["com.labwebsystem.installation-id"] != installation || labels["com.labwebsystem.app-id"] != app {
		return fmt.Errorf("LWS所有確認に失敗しました")
	}
	return nil
}
func ProjectName(app string) string { app = strings.ReplaceAll(app, "-", ""); return "lws-app-" + app }
