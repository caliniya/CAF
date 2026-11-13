package CreateMemoryToolInMemoryTool

import "github.com/caliniya/caf/core/Agent/AgentTool"

type CreateMemory struct {
	AgentTool.ToolMeta
}

// Fun implements [AgentTool.Tool].
func (c *CreateMemory) Fun(*AgentTool.ToolContext, map[string]any) (string, error) {
	panic("unimplemented")
}

// Load implements [AgentTool.Tool].
func (c *CreateMemory) Load() {
	panic("unimplemented")
}

// Meta implements [AgentTool.Tool].
func (c *CreateMemory) Meta() *AgentTool.ToolMeta {
	panic("unimplemented")
}
