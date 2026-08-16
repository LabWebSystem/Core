package backend

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

func OpenDB(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("データベースを開けません: %w", err)
	}
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("データベースを初期化できません: %w", err)
	}
	if _, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration管理表を作成できません: %w", err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("migration一覧を読み込めません: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", name).Scan(&applied); err != nil {
			db.Close()
			return nil, fmt.Errorf("migration状態を確認できません: %w", err)
		}
		if applied != 0 {
			continue
		}
		contents, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("migrationを読み込めません: %w", err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("migration transactionを開始できません: %w", err)
		}
		if _, err = tx.ExecContext(ctx, string(contents)); err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", name)
		}
		if err != nil {
			_ = tx.Rollback()
			db.Close()
			return nil, fmt.Errorf("データベースmigrationに失敗しました: %w", err)
		}
		if err = tx.Commit(); err != nil {
			db.Close()
			return nil, fmt.Errorf("データベースmigrationを確定できません: %w", err)
		}
	}
	return db, nil
}

func MarkUnfinishedOperations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `UPDATE operations SET state='FAILED', error_message='Backend再起動により未完了Operationを終了しました', updated_at=datetime('now') WHERE state IN ('QUEUED','RUNNING')`)
	return err
}

func sqlitePragmas(ctx context.Context, db *sql.DB) (foreignKeys, wal bool, err error) {
	var fk, journal string
	err = db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		return
	}
	err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal)
	return fk == "1", strings.EqualFold(journal, "wal"), err
}
