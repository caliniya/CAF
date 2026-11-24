package MemoryTool

import (
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	create "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/CreateMemory"
	list "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/ListMemories"
	read "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/ReadMemory"
	search "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/SearchMemory"
)

const SetName = "长期记忆工具"

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

func InitMemoryTool() *MainTools {

	return &MainTools{
		ToolSetMeta: &agenttool.ToolSetMeta{
			Name:        SetName,
			Description: "内置的记忆相关工具",
			Enabled:     true,
			ToolKits: map[string]agenttool.ToolKit{
				"创建记忆": create.InitCreateMemory(),
				"读取记忆": read.InitReadMemory(),
				"搜索记忆": search.InitSearchMemory(),
				"列出记忆": list.InitListMemories(),
			},
		},
	}
}
