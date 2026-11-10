package InteractionToolInMainTool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type AskUser struct{ agenttool.ToolMeta }

func (t *AskUser) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID: "AskUser", Name: "向用户提问",
		Description: "向用户提出一个问题，等待用户回答后返回回答文本。question 是完整的问题；options 是可选的建议答案，用户始终可以自由输入。用于澄清需求或让用户选择方案。默认等待 30 分钟，可通过 timeout_seconds 调整。",
		DisplayType: "question", DisplayContent: "arguments",
		Permission: agenttool.PermissionReadOnly, Enabled: true,
		Timeout: 30 * time.Minute, TimeoutParam: "timeout_seconds",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string", "minLength": 1, "description": "向用户展示的完整问题。"},
				"options":  map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1}, "description": "可选的建议答案；用户也可以自由输入。"},
			},
			"required": []string{"question"},
		},
	}
}

func (t *AskUser) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func (t *AskUser) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	question, ok := args["question"].(string)
	if !ok || strings.TrimSpace(question) == "" {
		return "", errors.New("参数 question 必须是非空字符串")
	}
	request := agenttool.UserQuestion{Question: question}
	if raw, present := args["options"]; present {
		var values []any
		switch options := raw.(type) {
		case []any:
			values = options
		case []string:
			for _, option := range options {
				values = append(values, option)
			}
		default:
			return "", errors.New("参数 options 必须是非空字符串数组")
		}
		if len(values) == 0 {
			return "", errors.New("参数 options 不能为空数组")
		}
		seen := make(map[string]bool, len(values))
		for i, value := range values {
			option, ok := value.(string)
			option = strings.TrimSpace(option)
			if !ok || option == "" || seen[option] {
				return "", fmt.Errorf("参数 options 的第 %d 项必须是非空且不重复的字符串", i+1)
			}
			seen[option] = true
			request.Options = append(request.Options, option)
		}
	}
	if ctx == nil || ctx.AskUser == nil {
		return "", errors.New("当前宿主未提供用户提问交互")
	}
	callContext := ctx.Context
	if callContext == nil {
		callContext = context.Background()
	}
	return ctx.AskUser(callContext, request)
}
