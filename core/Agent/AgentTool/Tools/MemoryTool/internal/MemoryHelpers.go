package memoryutil

import (
	"context"
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

func LibraryParameter() map[string]any {
	return map[string]any{"type": "string", "description": "当前对话已启用的记忆库 ID"}
}

func ReadContext(ctx *agenttool.ToolContext) (context.Context, error) {
	if ctx == nil || !ctx.Check(agenttool.PermissionReadOnly) {
		return nil, errors.New("没有读取记忆所需的权限")
	}
	parent := ctx.Context
	if parent == nil {
		parent = context.Background()
	}
	return parent, parent.Err()
}

func StringArgument(args map[string]any, name string) (string, error) {
	value, ok := args[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", errors.New(name + " 必须是非空字符串")
	}
	return strings.TrimSpace(value), nil
}
