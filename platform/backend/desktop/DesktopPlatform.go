package DesktopBackend

import (
	"context"
	"errors"
	"io/fs"
	"os"

	backend "github.com/caliniya/caf/core/Backend"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func Init(assets fs.FS) error {
	if assets == nil {
		return errors.New("必须提供前端")
	}
	fileSystem := &DesktopFileSystem{}
	backend.Platform.FS = fileSystem

	path, err := DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
	if err != nil {
		return err
	}
	backend.Platform.Paths = &path
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		return err
	}

	return wails.Run(&options.App{
		Title: "CAF", Width: 980, Height: 680, MinWidth: 680, MinHeight: 500,
		BackgroundColour: &options.RGBA{R: 245, G: 246, B: 250, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			// 原生文件夹选择器需要桌面运行时上下文。
			fileSystem.ctx = ctx
			// 把事件推送能力注入 Backend，后端 API 包自身不依赖 Wails。
			backend.Emit = func(event string, data ...any) {
				runtime.EventsEmit(ctx, event, data...)
			}
			backend.Platform.StartExtensionClients()
		},
		OnShutdown: func(ctx context.Context) {
			backend.Platform.CloseQQAdapters()
		},
		Bind: []interface{}{
			backend.Platform,
		},
	})
}
