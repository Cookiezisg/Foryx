// Package main is the backend server entrypoint: a thin shell over bootstrap.Build (the real DI
// composition root). It reads config from the environment, wires SIGINT/SIGTERM to a context, and
// hands off to App.Serve — which owns boot, serving, and the ordered graceful shutdown. The shell
// knows nothing about the shutdown sequence; that is the backend's own feature.
//
// backend 服务入口：bootstrap.Build 的薄壳。从环境读配置、把 SIGINT/SIGTERM 接成 ctx，交给 App.Serve——
// 它拥有 boot、服务、有序优雅关停。壳不懂关停顺序，那是 backend 自己的功能。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	bootstrappkg "github.com/sunweilin/anselm/backend/internal/bootstrap"
)

func main() {
	app, err := bootstrappkg.Build(bootstrappkg.Config{
		DataDir:   dataDir(),
		Addr:      os.Getenv("ANSELM_ADDR"),       // "" → 127.0.0.1:8080 (loopback-only)
		AuthToken: os.Getenv("ANSELM_AUTH_TOKEN"), // "" → bearer enforcement off (dev / testend)
		Dev:       os.Getenv("ANSELM_DEV") != "",
	})
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	// SIGINT/SIGTERM cancels ctx; App.Serve boots background work, serves HTTP, and runs the ordered
	// graceful shutdown (SSE streams → HTTP drain → background → DB) when ctx is cancelled.
	//
	// SIGINT/SIGTERM 取消 ctx；App.Serve 启后台、服务 HTTP，并在 ctx 取消时跑有序优雅关停（SSE 流 → HTTP
	// 排空 → 后台 → DB）。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := app.Serve(ctx); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// dataDir resolves the local data root: $ANSELM_DATA_DIR, else ~/.anselm.
//
// dataDir 解析本地数据根：$ANSELM_DATA_DIR，否则 ~/.anselm。
func dataDir() string {
	if d := os.Getenv("ANSELM_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".anselm"
	}
	return filepath.Join(home, ".anselm")
}
