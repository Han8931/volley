package main

// Volley — the desktop app: the same request engine and stores as the TUI in
// a native window (Wails v2; WebKit on macOS, WebKitGTK on Linux). Build with
// `wails build` from this directory; the TUI is untouched.

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/loadtest"
	"github.com/tabularasa/volley/internal/vars"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := newApp(collections.DefaultStore(), vars.DefaultEnvStore(),
		loadtest.DefaultStore(), loadtest.DefaultResultStore())

	if err := wails.Run(&options.App{
		Title:  "Volley",
		Width:  1180,
		Height: 780,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: a.startup,
		Bind:      []interface{}{a},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "volley:", err)
		os.Exit(1)
	}
}
