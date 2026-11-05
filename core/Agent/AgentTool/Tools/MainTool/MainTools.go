package MainTool

import agenttool "github.com/caliniya/caf/core/Agent/AgentTool"

//import agenttool "github.com/caliniya/caf/core/Agent/AgentTool"

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

// ListMap implements [agenttool.ToolsSet].
func (m *MainTools) ListMap() map[string]agenttool.ToolKit {
	return m.ToolKits
}

// Load implements [agenttool.ToolsSet].
func (m *MainTools) Load() {
	m.ToolSetMeta = &agenttool.ToolSetMeta{}
}

// Meta implements [agenttool.ToolsSet].
func (m *MainTools) Meta() agenttool.ToolSetMeta {
	return *m.ToolSetMeta
}
