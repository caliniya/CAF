package apiImpl

// 表示一个工具
type ChatTool struct {
	Name        string
	Parameters  map[string]any
	Description string
}

// 表示一条对话消息
type ChatMessage struct {
	Type string
	Text string
	Name string
}

type APIStruct interface {
	SetTool(*[]ChatTool) int
}
