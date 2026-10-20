package window

import (
	"errors"
	"io/fs"

	"github.com/caliniya/caf/core"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func Run(assets fs.FS, engine *core.Core) error {
	if assets == nil || engine == nil {
		return errors.New("必须提供前端资源和核心实例")
	}
	return wails.Run(&options.App{
		Title: "CAF", Width: 980, Height: 680, MinWidth: 680, MinHeight: 500,
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        engine.Startup,
		OnShutdown:       engine.Shutdown,
		Bind:             []interface{}{engine},
	})
}
