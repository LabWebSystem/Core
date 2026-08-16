package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	domainPattern  = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?)+$`)
)

type application struct {
	paths          paths
	version        string
	domain         string
	installationID string
	publicAddress  string
}

func newApplication() (*application, error) {
	p := defaultPaths()
	contents, err := os.ReadFile(p.versionFile)
	if err != nil {
		return nil, fmt.Errorf("LWSバージョンファイルを読み取れません: %s", p.versionFile)
	}
	version := strings.TrimSpace(string(contents))
	if !versionPattern.MatchString(version) {
		return nil, fmt.Errorf("LWSバージョンが不正です: %s", version)
	}
	return &application{paths: p, version: version}, nil
}

func (a *application) configFile() string { return filepath.Join(a.paths.configDir, "config.env") }

func (a *application) loadConfig() error {
	file, err := os.Open(a.configFile())
	if err != nil {
		return err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok || (key != "LWS_BASE_DOMAIN" && key != "LWS_VERSION" && key != "LWS_INSTALLATION_ID" && key != "LWS_PUBLIC_ADDRESS") {
			return errors.New("LWSの設定が不正です")
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !domainPattern.MatchString(values["LWS_BASE_DOMAIN"]) || !versionPattern.MatchString(values["LWS_VERSION"]) {
		return errors.New("LWSの設定が不正です")
	}
	a.domain = values["LWS_BASE_DOMAIN"]
	a.installationID = values["LWS_INSTALLATION_ID"]
	a.publicAddress = values["LWS_PUBLIC_ADDRESS"]
	needsMigration := a.installationID == "" || a.publicAddress == ""
	if a.installationID == "" {
		a.installationID = uuid.NewString()
	}
	if a.publicAddress == "" {
		address, err := detectPublicIPv4()
		if err != nil {
			return err
		}
		a.publicAddress = address
	}
	if needsMigration {
		if err := a.writeConfig(a.domain); err != nil {
			return fmt.Errorf("LWS設定を移行できません: %w", err)
		}
	}
	return nil
}

func (a *application) writeConfig(domain string) error {
	if err := os.MkdirAll(a.paths.configDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(a.paths.stateDir, 0o755); err != nil {
		return err
	}
	if a.installationID == "" {
		a.installationID = uuid.NewString()
	}
	if a.publicAddress == "" {
		address, err := detectPublicIPv4()
		if err != nil {
			return err
		}
		a.publicAddress = address
	}
	contents := fmt.Sprintf("LWS_BASE_DOMAIN=%s\nLWS_VERSION=%s\nLWS_INSTALLATION_ID=%s\nLWS_PUBLIC_ADDRESS=%s\n", domain, a.version, a.installationID, a.publicAddress)
	temporary, err := os.CreateTemp(a.paths.configDir, ".config.env-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, a.configFile()); err != nil {
		return err
	}
	a.domain = domain
	return nil
}

func detectPublicIPv4() (string, error) {
	if value := os.Getenv("LWS_PUBLIC_ADDRESS"); value != "" {
		ip := net.ParseIP(value)
		if ip != nil && ip.To4() != nil {
			return value, nil
		}
		return "", errors.New("公開IPv4アドレスが不正です")
	}
	if output, err := exec.Command("ip", "-4", "route", "get", "8.8.8.8").Output(); err == nil {
		fields := strings.Fields(string(output))
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "src" {
				ip := net.ParseIP(fields[i+1])
				if ip != nil && ip.To4() != nil {
					return fields[i+1], nil
				}
			}
		}
	}
	return "", errors.New("公開IPv4アドレスを一意に決定できません")
}
