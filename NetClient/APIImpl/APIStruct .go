package apiImpl

//表示一个工具
type ChatTool struct {
	Name        string
	Parameters  map[string]any
	Description string
}

type ChatMessage struct {
	Type string
}

type APIStruct interface {
	SetTool(*[]ChatTool) int
}
