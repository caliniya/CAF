package chat

import (
	"github.com/caliniya/caf/core"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

// ChatTask 保留旧包路径，实际类型统一定义在 core 包。
type ChatTask = core.ChatTask

func CreateChatTask(id, title, configID, model string, tree *chatstruct.ChatTree) *ChatTask {
	return core.NewChatTask(id, title, configID, model, tree)
}
