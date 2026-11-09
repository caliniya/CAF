package Onebot11QQ

import "strings"

// Command 只描述明确的指令，不将普通消息转换为模型请求。
type Command struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

// ParseCommand 接受 /扩展名称 指令；允许开头引用或 @ 当前账号。
// 自身消息、图片等非文本正文、@其他账号及普通聊天均忽略。
func ParseCommand(message QQMessage, name string) (Command, bool) {
	if message.SentBySelf || name == "" || strings.ContainsAny(name, "/ \t\r\n") {
		return Command{}, false
	}
	var text strings.Builder
	started := false
	for _, segment := range message.Segments {
		switch segment.Type {
		case QQSegmentText:
			value, ok := segment.Data["text"].(string)
			if !ok {
				return Command{}, false
			}
			text.WriteString(value)
			if strings.TrimSpace(value) != "" {
				started = true
			}
		case QQSegmentReply:
			if started {
				return Command{}, false
			}
		case QQSegmentAt:
			if started || message.SelfID == "" || segment.Data["qq"] != message.SelfID {
				return Command{}, false
			}
		default:
			return Command{}, false
		}
	}
	parts := strings.Fields(text.String())
	if len(parts) == 0 || parts[0] != "/"+name {
		return Command{}, false
	}
	if len(parts) == 1 {
		return Command{Name: "help", Args: []string{}}, true
	}
	return Command{Name: parts[1], Args: parts[2:]}, true
}
