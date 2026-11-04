package desktopBackend

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/caliniya/caf/core"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func Init(assets fs.FS) error {
	if assets == nil {
		return errors.New("必须提供前端")
	}
	core.Platform.FS = &DesktopFileSystem{}

	path, err := DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
	if err == nil {
		core.Platform.Paths = &path

		core.Platform.Paths.ConfigFile = filepath.Join(
			core.Platform.Paths.Data, "config", "MainConfig.json")
	} else {
		return err
	}

	DesktopQQPlatform = &DesktopQQ{}

	DesktopQQPlatform.CreateQQPlatform()

	return wails.Run(&options.App{
		Title: "CAF", Width: 980, Height: 680, MinWidth: 680, MinHeight: 500,
		BackgroundColour: &options.RGBA{R: 245, G: 246, B: 250, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			// 把事件推送能力注入 core,core 自身不依赖 wails。
			core.Emit = func(event string, data ...any) {
				runtime.EventsEmit(ctx, event, data...)
			}
		},
		Bind: []interface{}{
			core.Platform,
		},
	})
}
