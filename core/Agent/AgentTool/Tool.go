package agenttool

//一个工具的完整信息
type ToolMeta struct {
	Name        string
	Description string
	Parameters  map[string]any

	//对应的方法实现
	Fun ToolFunc
}

type ToolFunc func(map[string]any) (string, error)
