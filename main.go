package main

import (
	"embed"
	"io/fs"
	"log"

	core "github.com/caliniya/caf/core"
	agent "github.com/caliniya/caf/core/Agent"
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
	// 在启动期注册所有工具集，之后 AgentTool.ToolSets / Tools / ToolList 即可用。
	// 放在 main 而不是 core.Init()，避免 core → Agent → FileTool → core 的 import 环。
	agent.InitTool()

	if err := desktopBackend.Init(assets); err != nil {
		log.Fatal(err)
	}

	log.Printf("数据目录：%s", core.Platform.Paths.Data)

	if core.Platform != nil {
		log.Printf("QQ 后端类型：%T", core.Platform.QQPlatform)
	}

}
