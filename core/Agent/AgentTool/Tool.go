package agenttool

//一个工具的完整信息
type ToolMeta struct {
	Name        string
	Description string
	Parameters  map[string]any

	//对应的方法实现
	Fun ToolFunc
}

type Tool interface {
	Load()
	Name() string
	Description() string
	Fun(map[string]any) (string, error)
}

//某种工具集的表示
type ToolsSet struct {
	Name        string
	Description string
	Tools       []ToolMeta
}

//一套组合好的集
type ToolKit struct {
	Name        string
	Description string
	ToolSets    []ToolsSet
}

type ToolFunc func(map[string]any) (string, error)

type ToolKitF interface {
	EnableSet(string) error
	DisEnableSet(string) error

	Enable() error
	DisEnable() error
}
