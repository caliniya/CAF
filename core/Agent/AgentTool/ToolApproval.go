package AgentTool

import "context"

const RequestToolExecutionID = "RequestToolExecution"

// ToolGrant 仅属于保存它的对话。工作区授权按工作区 ID 匹配。
type ToolGrant struct {
	Scope       string `json:"scope"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

const (
	GrantWorkspace = "workspace"
	GrantTool      = "tool"
)

// ToolPermission 在执行前根据参数判断权限；Workspace 表示本次操作可以约束在工作区内。
type ToolPermission struct {
	Level     PermissionLevel
	Workspace bool
	Reason    string
}

type PermissionAwareTool interface {
	PermissionFor(*ToolContext, map[string]any) (ToolPermission, error)
}

// ExecutionRequester 由宿主提供，只接受本次生成中已经被权限检查拦截的调用 ID。
type ExecutionRequester func(context.Context, string) (string, error)
