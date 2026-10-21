package main

import (
	"embed"
	"io/fs"
	"log"

	desktopBackend "github.com/caliniya/caf/backend/desktop"
)

//go:embed frontend
var frontend embed.FS

func main() {
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	if err := desktopBackend.Run(assets); err != nil {
		log.Fatal(err)
	}
}
