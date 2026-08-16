package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/LabWebSystem/Core/backend"
)

func main() {
	path := os.Getenv("LWS_DATABASE_PATH")
	if path == "" {
		path = "/var/lib/lws/database.sqlite"
	}
	if err := backend.ValidateBaseDomain(os.Getenv("LWS_BASE_DOMAIN")); err != nil {
		slog.Error("ベースドメイン設定失敗", "error", err)
		os.Exit(1)
	}
	db, err := backend.OpenDB(context.Background(), path)
	if err != nil {
		slog.Error("データベース初期化失敗", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := backend.MarkUnfinishedOperations(context.Background(), db); err != nil {
		slog.Error("Operation整理失敗", "error", err)
		os.Exit(1)
	}
	addr := os.Getenv("LWS_LISTEN_ADDRESS")
	if addr == "" {
		addr = ":8080"
	}
	slog.Info("Backendを起動しました", "address", addr)
	appsRoot := os.Getenv("LWS_APPS_ROOT")
	if appsRoot == "" {
		appsRoot = "/var/lib/lws/apps"
	}
	if err := os.MkdirAll(appsRoot, 0700); err != nil {
		slog.Error("アプリ領域を作成できません", "error", err)
		os.Exit(1)
	}
	if err := http.ListenAndServe(addr, backend.NewServer(db, backend.NewRuntimeExecutor(db, appsRoot).Run).Handler()); err != nil {
		slog.Error("Backend停止", "error", err)
		os.Exit(1)
	}
}
