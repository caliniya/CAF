package desktopBackend

import (
	"errors"
	"io/fs"
	"os"

	"github.com/caliniya/caf/core"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func Run(assets fs.FS) error {
	if assets == nil {
		return errors.New("必须提供前端")
	}
	path, err := DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
	if err == nil {
		core.Platform.Paths = &path
	} else {
		return err
	}
	return wails.Run(&options.App{
		Title: "CAF", Width: 980, Height: 680, MinWidth: 680, MinHeight: 500,
		BackgroundColour: &options.RGBA{R: 245, G: 246, B: 250, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
	})
}
