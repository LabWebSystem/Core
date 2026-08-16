package backend

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

type DerivedManager struct {
	DB            *sql.DB
	GeneratedDir  string
	BaseDomain    string
	PublicAddress string
}

func (m *DerivedManager) Sync(ctx context.Context) error {
	rows, err := m.DB.QueryContext(ctx, `SELECT id,subdomain FROM applications WHERE registration_state='ACTIVE' ORDER BY subdomain`)
	if err != nil {
		return err
	}
	defer rows.Close()
	apps := []PublishedApplication{}
	for rows.Next() {
		var id, sub string
		if err := rows.Scan(&id, &sub); err != nil {
			return err
		}
		apps = append(apps, PublishedApplication{AppID: id, Subdomain: sub, Service: "web", Port: 80})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.GeneratedDir, 0700); err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(m.GeneratedDir, "hosts"), []byte(GenerateHosts(m.BaseDomain, m.PublicAddress, apps)), 0600); err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(m.GeneratedDir, "Caddyfile"), []byte(GenerateCaddyfile(m.BaseDomain, apps)), 0600); err != nil {
		return err
	}
	return nil
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
