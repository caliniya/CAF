package apiImpl

// 表示一个工具
type ChatTool struct {
	Name        string
	Parameters  map[string]any
	Description string
}

// 表示一条对话消息
// 已移至 Types.go 的正式定义（ChatMessage），此处不再重复声明。

type APIStruct interface {
	SetTool(*[]ChatTool) int
}
