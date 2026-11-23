package ReadMemoryToolInMemoryTool

import (
	"context"
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type ReadMemory struct{ agenttool.ToolMeta }
type SearchMemory struct{ agenttool.ToolMeta }
type ListMemories struct{ agenttool.ToolMeta }

func libraryParameter() map[string]any {
	return map[string]any{"type": "string", "description": "当前对话已启用的记忆库 ID"}
}

func (t *ReadMemory) Load() {
	t.ToolMeta = agenttool.ToolMeta{ID: "ReadMemory", Name: "读取记忆", Enabled: true, Permission: agenttool.PermissionReadOnly, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "result",
		Description: "按记忆库 ID 和记忆单元 ID 读取一条当前长期记忆，包括名称、正文、时间和标签。只能读取当前对话已启用的库，不读取历史副本。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"recollection_id": libraryParameter(), "memory_id": map[string]any{"type": "string", "description": "要读取的记忆单元 ID"}}, "required": []string{"recollection_id", "memory_id"}}}
}

func (t *SearchMemory) Load() {
	t.ToolMeta = agenttool.ToolMeta{ID: "SearchMemory", Name: "搜索记忆", Enabled: true, Permission: agenttool.PermissionReadOnly, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "result",
		Description: "在当前对话启用的记忆库中按关键词搜索名称、正文和标签，不区分大小写。可用 recollection_id 限定一个已启用库；省略时搜索全部已启用库。返回按库 ID、记忆 ID 索引的当前记忆，不检索历史。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"recollection_id": libraryParameter(), "query": map[string]any{"type": "string", "description": "非空搜索关键词，匹配名称、内容或标签"}}, "required": []string{"query"}}}
}

func (t *ListMemories) Load() {
	t.ToolMeta = agenttool.ToolMeta{ID: "ListMemories", Name: "列出记忆", Enabled: true, Permission: agenttool.PermissionReadOnly, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "result",
		Description: "列出指定已启用记忆库的全部当前记忆，返回库名称、介绍及按记忆 ID 索引的数据。包含各条记忆的名称、正文、时间和标签，不读取历史副本。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{"recollection_id": libraryParameter()}, "required": []string{"recollection_id"}}}
}

func (t *ReadMemory) Meta() *agenttool.ToolMeta   { return &t.ToolMeta }
func (t *SearchMemory) Meta() *agenttool.ToolMeta { return &t.ToolMeta }
func (t *ListMemories) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func readContext(ctx *agenttool.ToolContext) (context.Context, error) {
	if ctx == nil || !ctx.Check(agenttool.PermissionReadOnly) {
		return nil, errors.New("没有读取记忆所需的权限")
	}
	parent := ctx.Context
	if parent == nil {
		parent = context.Background()
	}
	return parent, parent.Err()
}

func stringArgument(args map[string]any, name string) (string, error) {
	value, ok := args[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", errors.New(name + " 必须是非空字符串")
	}
	return strings.TrimSpace(value), nil
}

func (t *ReadMemory) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	parent, err := readContext(ctx)
	if err != nil {
		return "", err
	}
	if ctx.ReadMemory == nil {
		return "", errors.New("当前宿主未提供记忆读取能力")
	}
	library, err := stringArgument(args, "recollection_id")
	if err != nil {
		return "", err
	}
	id, err := stringArgument(args, "memory_id")
	if err != nil {
		return "", err
	}
	return ctx.ReadMemory(parent, library, id)
}

func (t *SearchMemory) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	parent, err := readContext(ctx)
	if err != nil {
		return "", err
	}
	if ctx.SearchMemory == nil {
		return "", errors.New("当前宿主未提供记忆搜索能力")
	}
	query, err := stringArgument(args, "query")
	if err != nil {
		return "", err
	}
	library := ""
	if _, present := args["recollection_id"]; present {
		library, err = stringArgument(args, "recollection_id")
		if err != nil {
			return "", err
		}
	}
	return ctx.SearchMemory(parent, library, []string{query})
}

func (t *ListMemories) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	parent, err := readContext(ctx)
	if err != nil {
		return "", err
	}
	if ctx.ListMemories == nil {
		return "", errors.New("当前宿主未提供记忆列表能力")
	}
	library, err := stringArgument(args, "recollection_id")
	if err != nil {
		return "", err
	}
	return ctx.ListMemories(parent, library)
}

type ReadMemoryKit struct{ *agenttool.ToolKitMeta }

func (t *ReadMemoryKit) Meta() agenttool.ToolKitMeta         { return *t.ToolKitMeta }
func (t *ReadMemoryKit) ListName() map[string]agenttool.Tool { return t.Tools }
func (t *ReadMemoryKit) List() []agenttool.Tool {
	return []agenttool.Tool{t.Tools["ReadMemory"], t.Tools["SearchMemory"], t.Tools["ListMemories"]}
}

func InitReadMemory() *ReadMemoryKit {
	tools := map[string]agenttool.Tool{}
	for _, tool := range []agenttool.Tool{&ReadMemory{}, &SearchMemory{}, &ListMemories{}} {
		tool.Load()
		tools[tool.Meta().ID] = tool
	}
	return &ReadMemoryKit{ToolKitMeta: &agenttool.ToolKitMeta{Name: "查询记忆", Description: "读取、搜索和列出已启用记忆库中的当前记忆", Enabled: true, Tools: tools}}
}
