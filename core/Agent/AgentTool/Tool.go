package agenttool

type ToolMeta struct {
	Name        string
	Description string

	//写规范点注意
	Parameters map[string]any
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
	Tools       map[string]ToolKit
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
	List() map[string]ToolKit
}

// 一个大工具集的小模块
type ToolKit interface {
	Enable() error
	DisEnable() error

	Name() string
	Description() string
}
