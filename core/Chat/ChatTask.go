package chat

import (
	netclient "github.com/caliniya/caf/NetClient"
	"github.com/caliniya/caf/core"
)

// 表示一个对话任务,这在ui里面表示为一个选项
type ChatTask struct {
	//此任务使用的客户端
	Client *netclient.APIClient
}

func CreateChatTask(cfg *core.Config) *ChatTask {
	return &ChatTask{}
}
