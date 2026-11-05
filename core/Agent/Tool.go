package Agent

import (
	"github.com/caliniya/caf/core/Agent/AgentTool"
	"github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
)

func InitTool() {
	AgentTool.Init()
	AgentTool.Add(MainTool.InitMainTool())
}
