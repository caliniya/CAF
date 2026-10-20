package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/platforms/desktop"
	"github.com/caliniya/caf/platforms/desktop/window"
)

//go:embed frontend
var frontend embed.FS

func main() {
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	// 怎么选取平台后端?
	backend := desktop.New()
	core.Platform = core.NewCore(backend)
	if err := window.Run(assets, core.Platform); err != nil {
		log.Fatal(err)
	}
}
