// Command desktop is the Wails v2 shell that hosts the Forgify frontend.
// It spawns the existing cmd/server backend as a child process, captures
// the port the backend chose (BACKEND_PORT=<n> stdout line), and exposes
// that port to the frontend via the GetBackendPort Wails binding.
//
// Command desktop 是 Wails v2 桌面壳。它把现有 cmd/server 作为子进程拉起，
// 抓取后端选定的端口（stdout 中的 BACKEND_PORT=<n> 行），通过 GetBackendPort
// binding 暴露给前端。
package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:embed
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Forgify",
		Width:  1440,
		Height: 900,
		MinWidth: 1024,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []any{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}

// Ensure the embedded FS path is referenced so go:embed pulls assets in.
var _ = context.TODO
