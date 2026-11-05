package AgentTool

import (
	"errors"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// 这里的是全局性的
var Tools map[string]Tool
var ToolSets map[string]ToolsSet
var ToolList map[string]apiImpl.Tool

func Init() {
	Tools = map[string]Tool{}
	ToolSets = map[string]ToolsSet{}
	ToolList = map[string]apiImpl.Tool{}
}

func Add(t ToolsSet) error {
	if ToolSets == nil {
		return errors.New("要加载的工具集为nil")
	}

	ToolSets[t.Meta().Name] = t

	for _, kit := range t.Meta().ToolKits {
		for _, tool := range kit.List() {
			name := tool.Meta().Name

			if _, exists := Tools[name]; exists {
				return errors.New("重复的工具: " + name)
			}

			Tools[name] = tool
		}
	}
	Sync()
	return nil
}

func Sync() {
	for t := range Tools {
		meta := Tools[t].Meta()
		ToolList[meta.Name] = apiImpl.Tool{
			Name:        meta.Name,
			Description: meta.Description,
			Parameters:  meta.Parameters,
		}
	}
}
