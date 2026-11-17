package CreateMemoryToolInMemoryTool

import (
	"context"
	"errors"
	"strings"

	"github.com/caliniya/caf/core/Agent/AgentTool"
)

type CreateMemory struct{ AgentTool.ToolMeta }

func (c *CreateMemory) Load() {
	c.ToolMeta = AgentTool.ToolMeta{
		ID: "CreateMemory", Name: "创建记忆", Enabled: true,
		Description: "在当前对话已启用的记忆库中创建一条长期记忆。使用记忆上下文中的库 ID。非完全访问权限不能写入全局库。",
		// 具体写权限由工具和宿主按库作用域判断，独立于文件工具的工作区路径。
		Permission:  AgentTool.PermissionReadOnly,
		DisplayType: "info", DisplayContent: "arguments.data",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recollection_id": map[string]any{"type": "string", "description": "当前对话已启用的目标记忆库 ID"},
				"data":            map[string]any{"type": "string", "description": "单条记忆的正文"},
				"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可选标签，用于关键词检索"},
			},
			"required": []string{"recollection_id", "data"},
		},
	}
}

func (c *CreateMemory) Meta() *AgentTool.ToolMeta { return &c.ToolMeta }

func (c *CreateMemory) Fun(ctx *AgentTool.ToolContext, args map[string]any) (string, error) {
	if ctx == nil || ctx.CreateMemory == nil {
		return "", errors.New("当前宿主未提供记忆写入能力")
	}
	if ctx.PermissionLevel != AgentTool.PermissionAutoEdit && ctx.PermissionLevel != AgentTool.PermissionAutoApprove && ctx.PermissionLevel != AgentTool.PermissionFullAccess {
		return "", errors.New("当前权限不允许创建记忆")
	}
	libraryID, ok := args["recollection_id"].(string)
	if !ok || strings.TrimSpace(libraryID) == "" {
		return "", errors.New("recollection_id 必须是非空字符串")
	}
	data, ok := args["data"].(string)
	if !ok || strings.TrimSpace(data) == "" {
		return "", errors.New("data 必须是非空字符串")
	}
	tags := []string{}
	if value, supplied := args["tags"]; supplied {
		var list []any
		switch v := value.(type) {
		case []any:
			list = v
		case []string:
			for _, item := range v {
				list = append(list, item)
			}
		default:
			return "", errors.New("tags 必须是字符串数组")
		}
		for _, item := range list {
			tag, ok := item.(string)
			if !ok || strings.TrimSpace(tag) == "" {
				return "", errors.New("标签必须是非空字符串")
			}
			tags = append(tags, strings.TrimSpace(tag))
		}
	}
	callCtx := ctx.Context
	if callCtx == nil {
		callCtx = context.Background()
	}
	if err := callCtx.Err(); err != nil {
		return "", err
	}
	key, err := ctx.CreateMemory(callCtx, libraryID, data, tags)
	if err != nil {
		return "", err
	}
	return "已创建记忆：" + key, nil
}

type CreateMemoryKit struct{ *AgentTool.ToolKitMeta }

func (c *CreateMemoryKit) Meta() AgentTool.ToolKitMeta         { return *c.ToolKitMeta }
func (c *CreateMemoryKit) ListName() map[string]AgentTool.Tool { return c.Tools }
func (c *CreateMemoryKit) List() []AgentTool.Tool              { return []AgentTool.Tool{c.Tools["CreateMemory"]} }

func InitCreateMemory() *CreateMemoryKit {
	tool := &CreateMemory{}
	tool.Load()
	return &CreateMemoryKit{ToolKitMeta: &AgentTool.ToolKitMeta{
		Name: "创建记忆", Description: "在已启用的记忆库中保存单条长期记忆", Enabled: true,
		Tools: map[string]AgentTool.Tool{"CreateMemory": tool},
	}}
}
