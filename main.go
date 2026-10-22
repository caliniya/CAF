package main

import (
	"embed"
	"io/fs"
	"log"

	desktopBackend "github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
)

//go:embed frontend
var frontend embed.FS

func main() {
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	core.Init()

	if err := desktopBackend.Run(assets); err != nil {
		log.Fatal(err)
	}

	log.Printf(core.Platform.Paths.Data)

	if core.Platform != nil {
		log.Printf("QQ 后端类型：%T", core.Platform.QQPlatform)
	}

}
