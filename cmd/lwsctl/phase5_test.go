package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfigIsPrivate(t *testing.T) {
	dir := t.TempDir()
	app := &application{
		paths:          paths{configDir: filepath.Join(dir, "etc"), stateDir: filepath.Join(dir, "state")},
		version:        "0.1.0",
		installationID: "installation",
		publicAddress:  "192.0.2.10",
	}
	if err := app.writeConfig("example.internal"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(app.configFile())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config.envの権限が公開されています: %o", info.Mode().Perm())
	}
}

func TestStartRejectsOccupiedRequiredPortsBeforeCompose(t *testing.T) {
	old := requiredPortProbe
	t.Cleanup(func() { requiredPortProbe = old })
	requiredPortProbe = func(network, address string) error {
		if network == "tcp" && address == ":53" {
			return errPortOccupied
		}
		return nil
	}
	app := &application{domain: "example.internal"}
	if err := app.checkRequiredPorts(); err == nil {
		t.Fatal("使用中の必須ポートを受け付けました")
	}
}
