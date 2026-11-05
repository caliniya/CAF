package main

import (
	"embed"
	"io/fs"
	"log"

	core "github.com/caliniya/caf/core"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
)

//go:embed all:frontend/dist
var frontend embed.FS

func main() {
	assets, err := fs.Sub(frontend, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	core.Init()

	if err := desktopBackend.Init(assets); err != nil {
		log.Fatal(err)
	}

	log.Printf("数据目录：%s", core.Platform.Paths.Data)

	if core.Platform != nil {
		log.Printf("QQ 后端类型：%T", core.Platform.QQPlatform)
	}

}
