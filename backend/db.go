package backend

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
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
	contents, err := migrations.ReadFile("migrations/001_initial.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.ExecContext(ctx, string(contents)); err != nil {
		db.Close()
		return nil, fmt.Errorf("データベースmigrationに失敗しました: %w", err)
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
