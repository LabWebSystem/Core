package main

import "os"

type paths struct {
	versionFile   string
	installerPath string
	configDir     string
	stateDir      string
	composeFile   string
	project       string
}

func defaultPaths() paths {
	return paths{
		versionFile:   environment("LWS_VERSION_FILE", "/usr/share/lws/version"),
		installerPath: environment("LWS_INSTALLER_PATH", "/usr/share/lws/install.sh"),
		configDir:     environment("LWS_CONFIG_DIR", "/etc/lws"),
		stateDir:      environment("LWS_STATE_DIR", "/var/lib/lws"),
		composeFile:   environment("LWS_COMPOSE_FILE", "/usr/share/lws/compose.yaml"),
		project:       environment("LWS_COMPOSE_PROJECT", "lws"),
	}
}

func environment(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
