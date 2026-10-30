// テスト専用Backend。利用者環境の設定を読み込まず、一時領域だけを使用する。
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/LabWebSystem/Core/backend"
)

func main() {
	root := os.Getenv("LWS_E2E_ROOT")
	installation := os.Getenv("LWS_E2E_INSTALLATION")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	db, err := backend.OpenDB(context.Background(), filepath.Join(root, "database.sqlite"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	runtime := backend.NewRuntimeExecutor(db, filepath.Join(root, "apps"))
	runtime.InstallationID = installation
	runtime.Docker = backend.NewDockerResources(runtime.Runner, installation)
	runtime.Docker.CaddyContainer = os.Getenv("LWS_E2E_CADDY")
	server := backend.NewServer(db, runtime.Run)
	server.AppsRoot = runtime.Root
	server.Docker = runtime.Docker
	server.DeviceScanner = nil
	server.AllowedOrigin = os.Getenv("LWS_E2E_ORIGIN")
	if err := os.WriteFile(filepath.Join(root, "backend-address"), []byte("http://"+listener.Addr().String()), 0600); err != nil {
		panic(err)
	}
	if err := http.Serve(listener, server.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
