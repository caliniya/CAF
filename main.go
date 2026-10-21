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
	// 仅保留可构建的窗口入口；不绑定业务对象，也不启动 QQ 或创建数据目录。
	if err := desktopBackend.Run(assets); err != nil {
		log.Fatal(err)
	}
}
