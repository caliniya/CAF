package chat

import (
	netclient "github.com/caliniya/caf/NetClient"
	"github.com/caliniya/caf/core"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

// 表示一个对话任务,这在ui里面表示为一个选项
type ChatTask struct {
	//此任务使用的客户端
	Client *netclient.APIClient
	//此任务的对话树
	Tree *chatstruct.ChatTree
}

func CreateChatTask(cfg *core.Config) *ChatTask {
	return &ChatTask{}
}
