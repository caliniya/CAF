package MemoryTool

import (
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type MainTools struct {
	*agenttool.ToolSetMeta
}

// Get implements [agenttool.ToolsSet].
func (m *MainTools) Get(n string) agenttool.ToolKit {
	return m.ToolKits[n]
}

// List implements [agenttool.ToolsSet].
func (m *MainTools) List() []agenttool.ToolKit {
	temp := []agenttool.ToolKit{}
	for _, v := range m.ToolKits {
		temp = append(temp, v)
	}
	return temp
}

// Meta implements [agenttool.ToolsSet].
func (m *MainTools) Meta() agenttool.ToolSetMeta {
	return *m.ToolSetMeta
}

func InitMainTool() *MainTools {

	return &MainTools{
		ToolSetMeta: &agenttool.ToolSetMeta{
			Name:        "长期记忆工具",
			Description: "内置的记忆相关工具",
			Enabled:     true,
			ToolKits:    map[string]agenttool.ToolKit{},
		},
	}
}
