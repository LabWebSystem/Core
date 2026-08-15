package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	domainPattern  = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?)+$`)
)

type application struct {
	paths   paths
	version string
	domain  string
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
		if !ok || (key != "LWS_BASE_DOMAIN" && key != "LWS_VERSION") {
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
	return nil
}

func (a *application) writeConfig(domain string) error {
	if err := os.MkdirAll(a.paths.configDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(a.paths.stateDir, 0o755); err != nil {
		return err
	}
	contents := fmt.Sprintf("LWS_BASE_DOMAIN=%s\nLWS_VERSION=%s\n", domain, a.version)
	if err := os.WriteFile(a.configFile(), []byte(contents), 0o644); err != nil {
		return err
	}
	a.domain = domain
	return nil
}
