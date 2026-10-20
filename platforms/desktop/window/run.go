// Package window owns Wails-specific window setup. The desktop backend/facade
// remains importable for headless tests without linking Wails/GTK.
package window

import (
	"errors"
	"io/fs"

	"github.com/caliniya/caf/platforms/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func Run(assets fs.FS, app *desktop.App) error {
	if assets == nil || app == nil {
		return errors.New("assets and desktop app are required")
	}
	return wails.Run(&options.App{
		Title: "CAF", Width: 980, Height: 680, MinWidth: 680, MinHeight: 500,
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Bind:             []interface{}{app},
	})
}
