package SearchMemoryToolInMemoryTool

import (
	"errors"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	memoryutil "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/internal"
)

type SearchMemory struct{ agenttool.ToolMeta }

func (t *SearchMemory) Load() {
	t.ToolMeta = agenttool.ToolMeta{ID: "SearchMemory", Name: "搜索记忆", Enabled: true, Permission: agenttool.PermissionReadOnly, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "result",
		Description: "在当前对话启用的记忆库中按关键词搜索名称、正文和标签，不区分大小写。可用 recollection_id 限定一个已启用库；省略时搜索全部已启用库。返回按库 ID、记忆 ID 索引的当前记忆，不检索历史。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"recollection_id": memoryutil.LibraryParameter(), "query": map[string]any{"type": "string", "description": "非空搜索关键词，匹配名称、内容或标签"}}, "required": []string{"query"}}}
}

func (t *SearchMemory) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func (t *SearchMemory) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	parent, err := memoryutil.ReadContext(ctx)
	if err != nil {
		return "", err
	}
	if ctx.SearchMemory == nil {
		return "", errors.New("当前宿主未提供记忆搜索能力")
	}
	query, err := memoryutil.StringArgument(args, "query")
	if err != nil {
		return "", err
	}
	library := ""
	if _, present := args["recollection_id"]; present {
		library, err = memoryutil.StringArgument(args, "recollection_id")
		if err != nil {
			return "", err
		}
	}
	return ctx.SearchMemory(parent, library, []string{query})
}

type SearchMemoryKit struct{ *agenttool.ToolKitMeta }

func (t *SearchMemoryKit) Meta() agenttool.ToolKitMeta         { return *t.ToolKitMeta }
func (t *SearchMemoryKit) ListName() map[string]agenttool.Tool { return t.Tools }
func (t *SearchMemoryKit) List() []agenttool.Tool              { return []agenttool.Tool{t.Tools["SearchMemory"]} }

func InitSearchMemory() *SearchMemoryKit {
	tool := &SearchMemory{}
	tool.Load()
	return &SearchMemoryKit{ToolKitMeta: &agenttool.ToolKitMeta{
		Name: "搜索记忆", Description: "按关键词搜索已启用记忆库中的当前记忆", Enabled: true,
		Tools: map[string]agenttool.Tool{"SearchMemory": tool},
	}}
}
