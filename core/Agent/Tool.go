package Agent

import (
	"log"

	"github.com/caliniya/caf/core/Agent/AgentTool"
	"github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
)

// InitTool 只在启动期调用一次。工具集是静态的，不做热重载。
// 注册失败（工具重名、kit 结构异常等）直接终止进程，
// 避免带着残缺的工具表继续运行——那种情况在界面上几乎查不出来。
func InitTool() {
	AgentTool.Init()
	if err := AgentTool.Add(MainTool.InitMainTool()); err != nil {
		log.Fatalf("工具注册失败：%v", err)
	}
}
