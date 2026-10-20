package main

import (
	"embed"
	"io/fs"
	"log"
	"os"

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
	// 入口只选择平台实现、解析平台目录；核心统一处理初始化、业务和生命周期。
	backend := desktop.New()
	paths, err := desktop.DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
	if err != nil {
		log.Fatal(err)
	}
	// 全局入口保存指针，窗口绑定同一实例，不能复制包含互斥锁的 Core 值。
	core.Platform, err = core.NewCore(paths, backend)
	if err != nil {
		log.Fatal(err)
	}
	if err := window.Run(assets, core.Platform); err != nil {
		log.Fatal(err)
	}
}
