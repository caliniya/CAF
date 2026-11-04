package agenttool

//方便内部使用的元结构体,内嵌使用
type ToolMeta struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Tool interface {
	Load()
	Meta() *ToolMeta
	Fun(map[string]any) (string, error)
}

// 某种工具集的表示
type ToolSet struct {
	Name        string
	Description string
	ToolKits    map[string]ToolKit
}

// 接口
type ToolsSet interface {
	Name() string
	Description() string

	EnableSet(string) error
	DisEnableSet(string) error

	Enable() error
	DisEnable() error

	Get(string) ToolKit
	ListMap() map[string]ToolKit
	List() []ToolKit
}

// 一个大工具集的小模块
type ToolKit interface {
	Enable() error
	DisEnable() error

	Name() string
	Description() string

	ListName() map[string]Tool
	List() []Tool
}
