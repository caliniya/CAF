package DesktopBackend

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

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
	backend.Platform.DeviceKeys = deviceKeys{}

	path, err := DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
	if err != nil {
		return err
	}
	backend.Platform.Paths = &path
	profiles := backend.NewProfiles(path, fileSystem, deviceKeys{}, backend.Platform.APIClient)

	timeContext, stopTime := context.WithCancel(context.Background())
	defer stopTime()
	var startTime sync.Once

	// Serialize registrations and writes for one data directory across desktop processes.
	instancePath := path.Data
	if real, err := filepath.EvalSymlinks(instancePath); err == nil {
		instancePath = real
	}
	if goruntime.GOOS == "windows" {
		instancePath = strings.ToLower(instancePath)
	}
	return wails.Run(&options.App{
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: fmt.Sprintf("caf-%x", sha256.Sum256([]byte(instancePath))), OnSecondInstanceLaunch: func(options.SecondInstanceData) {}},
		Title:              "CAF", Width: 980, Height: 680, MinWidth: 680, MinHeight: 500,
		BackgroundColour: &options.RGBA{R: 245, G: 246, B: 250, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			// 原生文件夹选择器需要桌面运行时上下文。
			fileSystem.ctx = ctx
			// 把事件推送能力注入 Backend，后端 API 包自身不依赖 Wails。
			backend.Emit = func(event string, data ...any) {
				runtime.EventsEmit(ctx, event, data...)
			}
			profiles.Start(ctx)
		},
		OnDomReady: func(ctx context.Context) {
			// 前端重载时仍只保留一个取时循环；网络请求不阻塞窗口启动。
			startTime.Do(func() {
				go func() {
					for timeContext.Err() == nil {
						sample := backend.Platform.APIClient.TimeClient.Now(timeContext)
						if timeContext.Err() != nil {
							return
						}
						source := "在线"
						if sample.Source == "local" {
							source = "脱机"
						}
						runtime.WindowSetTitle(ctx, "CAF | "+sample.ToString()+" ["+source+"]")
						select {
						case <-timeContext.Done():
							return
						case <-time.After(100 * time.Millisecond):
						}
					}
				}()
			})
		},
		OnBeforeClose: func(ctx context.Context) bool {
			stopTime()
			return false
		},
		OnShutdown: func(ctx context.Context) {
			stopTime()
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := profiles.Close(closeCtx); err != nil {
				runtime.LogError(ctx, "关闭用户会话失败："+err.Error())
			}
		},
		Bind: []interface{}{
			profiles.API(),
		},
	})
}
