package main

import (
	"embed"
	"io/fs"
	"log"

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
	// Entry point selects the implementation. Constructors do not initialize it.
	backend := desktop.New()
	app := desktop.NewApp(backend)
	if err := window.Run(assets, app); err != nil {
		log.Fatal(err)
	}
}
