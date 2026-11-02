package agenttool

type Tool interface {
	Load()
	Name() string
	Description() string
	Fun(map[string]any) (string, error)
}

// 某种工具集的表示
type ToolsSet struct {
	Name        string
	Description string
	Tools       []Tool
}

// 一套组合好的集
type ToolKit struct {
	Name        string
	Description string
	ToolSets    []ToolsSet
}

type ToolKitF interface {
	EnableSet(string) error
	DisEnableSet(string) error

	Enable() error
	DisEnable() error
}
