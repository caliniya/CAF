package InteractionToolInMainTool

import "github.com/caliniya/caf/core/Agent/AgentTool"

type InteractionTool struct {
	*AgentTool.ToolKitMeta
}

func (t *InteractionTool) List() []AgentTool.Tool {
	tools := make([]AgentTool.Tool, 0, len(t.Tools))
	for _, tool := range t.Tools {
		tools = append(tools, tool)
	}
	return tools
}

func (t *InteractionTool) ListName() map[string]AgentTool.Tool { return t.Tools }
func (t *InteractionTool) Meta() AgentTool.ToolKitMeta         { return *t.ToolKitMeta }

func InitInteractionTool() *InteractionTool {
	tools := []AgentTool.Tool{&AskUser{}}
	registered := make(map[string]AgentTool.Tool, len(tools))
	for _, tool := range tools {
		tool.Load()
		registered[tool.Meta().ID] = tool
	}
	return &InteractionTool{ToolKitMeta: &AgentTool.ToolKitMeta{
		Name: "交互工具", Description: "标准工具集的一部分，提供与用户交互的能力",
		Enabled: true, Tools: registered,
	}}
}
