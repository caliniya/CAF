package InteractionToolInMainTool

import (
	"context"
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type RequestToolExecution struct{ agenttool.ToolMeta }

func (t *RequestToolExecution) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID: agenttool.RequestToolExecutionID, Name: "申请执行工具", Enabled: true,
		Description: "仅在严格模式中启用。工具因权限不足返回调用 ID 后，用 call_id 主动提交该调用的审批请求。用户批准后，宿主执行保存的原始调用，并通过本工具返回执行结果，无需再次调用原工具。用户拒绝后应调整方案。不能申请历史对话、未被拦截或已处理的调用。",
		Permission:  agenttool.PermissionReadOnly,
		DisplayType: "info", DisplayContent: "result",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"call_id": map[string]any{"type": "string", "minLength": 1, "description": "权限不足结果中提供的原始工具调用 ID"},
			},
			"required": []string{"call_id"},
		},
	}
}

func (t *RequestToolExecution) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func (t *RequestToolExecution) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	id, ok := args["call_id"].(string)
	if !ok || strings.TrimSpace(id) == "" {
		return "", errors.New("call_id 必须是非空字符串")
	}
	if ctx == nil || ctx.RequestToolExecution == nil {
		return "", errors.New("当前对话未启用严格模式申请")
	}
	parent := ctx.Context
	if parent == nil {
		parent = context.Background()
	}
	return ctx.RequestToolExecution(parent, id)
}
