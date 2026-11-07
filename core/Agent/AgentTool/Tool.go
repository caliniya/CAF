package AgentTool

import (
	"maps"
	"time"

	"github.com/caliniya/caf/NetClient/APIImpl"
)

// 方便内部使用的元结构体,内嵌使用
type ToolMeta struct {
	ID          string
	Name        string
	Description string
	Parameters  map[string]any
	// Permission 是工具执行所需的最低权限；未设置时按只读处理。
	Permission PermissionLevel

	// Timeout 是工具默认超时；为 0 时使用统一的 30 秒。
	Timeout time.Duration
	// TimeoutParam 非空时，允许模型通过这个可选参数指定超时（单位：秒）。
	// 参数省略或为 0 时使用工具默认超时。
	TimeoutParam string

	Enabled bool
}

// ToType 生成模型可见的工具声明，按需补充超时参数，不修改原始 schema。
func (m *ToolMeta) ToType() *APIImpl.Tool {
	parameters := m.Parameters
	if m.TimeoutParam != "" {
		parameters = maps.Clone(parameters)
		if parameters == nil {
			parameters = map[string]any{"type": "object"}
		}
		properties, _ := parameters["properties"].(map[string]any)
		properties = maps.Clone(properties)
		if properties == nil {
			properties = make(map[string]any)
		}
		if _, exists := properties[m.TimeoutParam]; !exists {
			properties[m.TimeoutParam] = map[string]any{
				"type":        "number",
				"description": "本次调用的超时时间（秒）；省略或设为 0 时使用工具默认超时。",
				"minimum":     0,
				"default":     0,
			}
		}
		parameters["properties"] = properties
	}
	return &APIImpl.Tool{
		Name:        m.ID,
		Description: m.Description,
		Parameters:  parameters,
	}
}

type Tool interface {
	Meta() *ToolMeta
	Fun(*ToolContext, map[string]any) (string, error)

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
