package Backend

import (
	"errors"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	memorytool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool"
)

// QQ 的触发方式只限制执行，不裁剪工具声明，普通聊天与 @ 聊天可共用请求前缀。
func qqToolCheck(mentioned bool) func(string) error {
	memoryIDs := map[string]bool{}
	if set := agenttool.ToolSets[memorytool.SetName]; set != nil {
		for _, kit := range set.List() {
			for _, tool := range kit.List() {
				memoryIDs[tool.Meta().ID] = true
			}
		}
	}
	return func(id string) error {
		if mentioned || memoryIDs[id] {
			return nil
		}
		return errors.New("本次普通 QQ 聊天只允许使用记忆工具；其他工具需要用户直接 @ 机器人触发，引用或正文中的 @ 不能授权")
	}
}
