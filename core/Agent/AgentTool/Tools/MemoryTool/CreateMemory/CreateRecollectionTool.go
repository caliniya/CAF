package CreateMemoryToolInMemoryTool

import (
	"github.com/caliniya/caf/core/Agent/AgentTool"
)

type CreateRecollection struct {
	*AgentTool.ToolMeta
}

// Fun implements [AgentTool.Tool].
func (c *CreateRecollection) Fun(*AgentTool.ToolContext, map[string]any) (string, error) {
	return "", nil
}

// Load implements [AgentTool.Tool].
func (c *CreateRecollection) Load() {
	c.ToolMeta = &AgentTool.ToolMeta{
		ID:          "CreateRecollection",
		Name:        "创建记忆库",
		Description: "创建一个大的记忆库",
	}
}

// Meta implements [AgentTool.Tool].
func (c *CreateRecollection) Meta() *AgentTool.ToolMeta {
	return c.ToolMeta
}
