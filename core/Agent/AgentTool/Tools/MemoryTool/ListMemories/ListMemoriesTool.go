package ListMemoriesToolInMemoryTool

import (
	"errors"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	memoryutil "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/internal"
)

type ListMemories struct{ agenttool.ToolMeta }

func (t *ListMemories) Load() {
	t.ToolMeta = agenttool.ToolMeta{ID: "ListMemories", Name: "列出记忆", Enabled: true, Permission: agenttool.PermissionReadOnly, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "result",
		Description: "列出指定已启用记忆库的全部当前记忆，返回库名称、介绍及按记忆 ID 索引的数据。包含各条记忆的名称、正文、时间和标签，不读取历史副本。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"recollection_id": memoryutil.LibraryParameter()}, "required": []string{"recollection_id"}}}
}

func (t *ListMemories) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func (t *ListMemories) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	parent, err := memoryutil.ReadContext(ctx)
	if err != nil {
		return "", err
	}
	if ctx.ListMemories == nil {
		return "", errors.New("当前宿主未提供记忆列表能力")
	}
	library, err := memoryutil.StringArgument(args, "recollection_id")
	if err != nil {
		return "", err
	}
	return ctx.ListMemories(parent, library)
}

type ListMemoriesKit struct{ *agenttool.ToolKitMeta }

func (t *ListMemoriesKit) Meta() agenttool.ToolKitMeta         { return *t.ToolKitMeta }
func (t *ListMemoriesKit) ListName() map[string]agenttool.Tool { return t.Tools }
func (t *ListMemoriesKit) List() []agenttool.Tool              { return []agenttool.Tool{t.Tools["ListMemories"]} }

func InitListMemories() *ListMemoriesKit {
	tool := &ListMemories{}
	tool.Load()
	return &ListMemoriesKit{ToolKitMeta: &agenttool.ToolKitMeta{
		Name: "列出记忆", Description: "列出一个已启用记忆库中的全部当前记忆", Enabled: true,
		Tools: map[string]agenttool.Tool{"ListMemories": tool},
	}}
}
