package ReadMemoryToolInMemoryTool

import (
	"errors"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	memoryutil "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/internal"
)

type ReadMemory struct{ agenttool.ToolMeta }

func (t *ReadMemory) Load() {
	t.ToolMeta = agenttool.ToolMeta{ID: "ReadMemory", Name: "读取记忆", Enabled: true, Permission: agenttool.PermissionReadOnly, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "result",
		Description: "按记忆库 ID 和记忆单元 ID 读取一条当前长期记忆，包括名称、正文、时间和标签。只能读取当前对话已启用的库，不读取历史副本。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"recollection_id": memoryutil.LibraryParameter(), "memory_id": map[string]any{"type": "string", "description": "要读取的记忆单元 ID"}}, "required": []string{"recollection_id", "memory_id"}}}
}

func (t *ReadMemory) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func (t *ReadMemory) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	parent, err := memoryutil.ReadContext(ctx)
	if err != nil {
		return "", err
	}
	if ctx.ReadMemory == nil {
		return "", errors.New("当前宿主未提供记忆读取能力")
	}
	library, err := memoryutil.StringArgument(args, "recollection_id")
	if err != nil {
		return "", err
	}
	id, err := memoryutil.StringArgument(args, "memory_id")
	if err != nil {
		return "", err
	}
	return ctx.ReadMemory(parent, library, id)
}

type ReadMemoryKit struct{ *agenttool.ToolKitMeta }

func (t *ReadMemoryKit) Meta() agenttool.ToolKitMeta         { return *t.ToolKitMeta }
func (t *ReadMemoryKit) ListName() map[string]agenttool.Tool { return t.Tools }
func (t *ReadMemoryKit) List() []agenttool.Tool {
	return []agenttool.Tool{t.Tools["ReadMemory"]}
}

func InitReadMemory() *ReadMemoryKit {
	tool := &ReadMemory{}
	tool.Load()
	return &ReadMemoryKit{ToolKitMeta: &agenttool.ToolKitMeta{
		Name: "读取记忆", Description: "按 ID 读取已启用记忆库中的一条当前记忆", Enabled: true,
		Tools: map[string]agenttool.Tool{"ReadMemory": tool},
	}}
}
