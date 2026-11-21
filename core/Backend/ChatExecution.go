package Backend

import (
	"context"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
)

// 内部客户端复用同一生成流程，由宿主提供存储、工作区和交互入口。
// 普通桌面对话不传入此配置，继续使用原有 API。
type chatExecutionOptions struct {
	workspace     *chat.WorkSpace
	systemPrompt  string
	memoryContext string
	memoryScope   func(context.Context, string) (bool, error)
	createMemory  func(*agenttool.ToolContext, string, string, []string) (string, error)
	// 声明保持稳定，宿主可以按当前入口另外限制工具执行。
	checkTool func(string) error
	save      func(*chat.ChatTask) error
	// 每次模型请求完成后调用一次；nil 表示该请求没有上报用量。
	recordUsage func(*api.TokenUsage)
	askUser     func(context.Context, string, string, string, agenttool.UserQuestion) (string, error)
	approve     func(context.Context, string, string, string, *api.ToolApproval, func(string) (*agenttool.ChatToolConfig, error)) (string, error)
}

func (c *Core) saveExecutionChatLocked(directory string, session *chat.ChatTask, options *chatExecutionOptions) error {
	if options != nil && options.save != nil {
		return options.save(session)
	}
	return c.saveChatLocked(directory, session)
}

func firstExecutionOptions(options []*chatExecutionOptions) *chatExecutionOptions {
	if len(options) > 0 {
		return options[0]
	}
	return nil
}
