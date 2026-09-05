package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

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
	secretKeyPath := os.Getenv("LWS_SECRET_KEY_PATH")
	if secretKeyPath == "" {
		secretKeyPath = "/etc/lws/secret.key"
	}
	secretKey, err := backend.LoadSecretKey(secretKeyPath)
	if err != nil {
		slog.Error("secret鍵を初期化できません", "error", err)
		os.Exit(1)
	}
	runtime := backend.NewRuntimeExecutor(db, appsRoot)
	runtime.SecretKey = secretKey
	runtime.InstallationID = os.Getenv("LWS_INSTALLATION_ID")
	if err := backend.ValidateInstallationID(runtime.InstallationID); err != nil {
		slog.Error("installation ID設定失敗", "error", err)
		os.Exit(1)
	}
	runtime.Docker = backend.NewDockerResources(runtime.Runner, runtime.InstallationID)
	if caddy := os.Getenv("LWS_CADDY_CONTAINER"); caddy != "" {
		runtime.Docker.CaddyContainer = caddy
	}
	if domain, address := os.Getenv("LWS_BASE_DOMAIN"), os.Getenv("LWS_PUBLIC_ADDRESS"); address != "" {
		derived := &backend.DerivedManager{DB: db, GeneratedDir: "/var/lib/lws/generated", BaseDomain: domain, PublicAddress: address}
		if err := derived.Sync(context.Background()); err != nil {
			slog.Error("派生設定の初期生成に失敗しました", "error", err)
			os.Exit(1)
		}
		derived.Docker = runtime.Docker
		derived.CaddyContainer = runtime.Docker.CaddyContainer
		derived.CoreDNSContainer = os.Getenv("LWS_COREDNS_CONTAINER")
		runtime.Derived = derived
		go func() {
			for attempt := 0; attempt < 60; attempt++ {
				if err := runtime.ReconcileStartup(context.Background()); err == nil {
					slog.Info("起動時のDocker・派生設定再調整が完了しました")
					return
				} else if attempt == 59 {
					slog.Error("起動時のDocker・派生設定再調整に失敗しました", "error", err)
				}
				<-time.After(time.Second)
			}
		}()
	}
	server := backend.NewServer(db, runtime.Run)
	server.Docker = runtime.Docker
	server.SecretKey = secretKey
	server.AppsRoot = appsRoot
	server.Logs.SecretKey = secretKey
	server.AllowedHost = os.Getenv("LWS_ALLOWED_HOST")
	if server.AllowedHost == "" {
		server.AllowedHost = "api." + os.Getenv("LWS_BASE_DOMAIN")
	}
	server.AllowedOrigin = os.Getenv("LWS_ALLOWED_ORIGIN")
	if server.AllowedOrigin == "" {
		server.AllowedOrigin = "http://dashboard." + os.Getenv("LWS_BASE_DOMAIN")
	}
	if source, err := backend.NewMobyDockerRuntimeSource(runtime.InstallationID); err != nil {
		slog.Error("Dockerログ監視を初期化できません", "error", err)
	} else {
		monitor := backend.NewRuntimeMonitor(source, server.Logs, db, runtime.InstallationID, os.Getenv("LWS_BASE_DOMAIN"), secretKey)
		monitorContext, cancelMonitor := context.WithCancel(context.Background())
		defer cancelMonitor()
		go monitor.Run(monitorContext)
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			if err := server.Logs.RemoveExpired(context.Background(), 30*24*time.Hour, 100*1024*1024); err != nil {
				slog.Error("ログ保持期限の整理に失敗しました", "error", err)
			}
			<-ticker.C
		}
	}()
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		slog.Error("Backend停止", "error", err)
		os.Exit(1)
	}
}
