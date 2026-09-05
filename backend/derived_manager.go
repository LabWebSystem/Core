package backend

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

type DerivedManager struct {
	DB               *sql.DB
	GeneratedDir     string
	BaseDomain       string
	PublicAddress    string
	Docker           *DockerResources
	CaddyContainer   string
	CoreDNSContainer string
}

const (
	generatedDirMode = 0o755
	derivedFileMode  = 0o644
)

func (m *DerivedManager) Sync(ctx context.Context) error {
	rows, err := m.DB.QueryContext(ctx, `SELECT id,subdomain,COALESCE(NULLIF(public_service,''),manifest_service),CASE WHEN public_port > 0 THEN public_port ELSE manifest_port END FROM applications WHERE registration_state='ACTIVE' ORDER BY subdomain`)
	if err != nil {
		return err
	}
	defer rows.Close()
	apps := []PublishedApplication{}
	for rows.Next() {
		var id, sub, service string
		var port int
		if err := rows.Scan(&id, &sub, &service, &port); err != nil {
			return err
		}
		apps = append(apps, PublishedApplication{AppID: id, Subdomain: sub, Service: service, Port: port})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.GeneratedDir, generatedDirMode); err != nil {
		return err
	}
	if err := os.Chmod(m.GeneratedDir, generatedDirMode); err != nil {
		return err
	}
	hostsPath := filepath.Join(m.GeneratedDir, "hosts")
	caddyPath := filepath.Join(m.GeneratedDir, "Caddyfile")
	oldHosts, hostsExisted, err := readDerivedFile(hostsPath)
	if err != nil {
		return err
	}
	oldCaddy, caddyExisted, err := readDerivedFile(caddyPath)
	if err != nil {
		return err
	}
	restore := func() {
		_ = restoreDerivedFile(hostsPath, oldHosts, hostsExisted)
		_ = restoreDerivedFile(caddyPath, oldCaddy, caddyExisted)
	}
	if err := WriteAtomic(caddyPath, []byte(GenerateCaddyfile(m.BaseDomain, apps)), derivedFileMode); err != nil {
		return err
	}
	if m.Docker != nil {
		if m.CaddyContainer != "" {
			m.Docker.CaddyContainer = m.CaddyContainer
		}
		if err := m.Docker.VerifyInfrastructureContainer(ctx, m.Docker.CaddyContainer); err != nil {
			restore()
			return err
		}
		if err := m.Docker.VerifyInfrastructureContainer(ctx, m.CoreDNSContainer); err != nil {
			restore()
			return err
		}
		if err := m.Docker.ValidateCaddyfile(ctx); err != nil {
			restore()
			return err
		}
	}
	if err := WriteAtomic(hostsPath, []byte(GenerateHosts(m.BaseDomain, m.PublicAddress, apps)), derivedFileMode); err != nil {
		restore()
		return err
	}
	if m.Docker != nil {
		if err := m.Docker.ReloadCaddy(ctx); err != nil {
			restore()
			return err
		}
		if err := m.Docker.ReloadCoreDNS(ctx, m.CoreDNSContainer); err != nil {
			restore()
			_ = m.Docker.ReloadCaddy(ctx)
			return err
		}
	}
	return nil
}

func readDerivedFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func restoreDerivedFile(path string, data []byte, existed bool) error {
	if !existed {
		return os.Remove(path)
	}
	return WriteAtomic(path, data, derivedFileMode)
}
func (m *DerivedManager) Validate() error {
	if err := ValidateBaseDomain(m.BaseDomain); err != nil {
		return err
	}
	if m.PublicAddress == "" {
		return fmt.Errorf("公開アドレスが必要です")
	}
	return nil
}
