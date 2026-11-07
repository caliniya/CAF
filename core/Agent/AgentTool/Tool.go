package AgentTool

import "github.com/caliniya/caf/NetClient/APIImpl"

// 方便内部使用的元结构体,内嵌使用
type ToolMeta struct {
	ID          string
	Name        string
	Description string
	Parameters  map[string]any

	Enabled bool
}

type Tool interface {
	Meta() *ToolMeta
	Fun(map[string]any) (string, error)

	ToType() *APIImpl.Tool
}

// 某种工具集的表示
type ToolSetMeta struct {
	Name        string
	Description string
	ToolKits    map[string]ToolKit

	Enabled bool
}

// 接口
type ToolsSet interface {
	Load()

	Meta() ToolSetMeta

	Get(string) ToolKit
	ListMap() map[string]ToolKit
	List() []ToolKit
}

type ToolKitMeta struct {
	Name        string
	Description string

	Tools map[string]Tool

	Enabled bool
}

// 一个大工具集的小模块
type ToolKit interface {
	Meta() ToolKitMeta

	ListName() map[string]Tool
	List() []Tool
}
