package AgentTool

// 用于工具的一个简单上下文
type ToolContext struct {
	WorkSpace string
}

type PermissionLevel int

const (
	PermissionReadOnly    PermissionLevel = 1
	PermissionAutoEdit    PermissionLevel = 2
	PermissionAutoApprove PermissionLevel = 3
	PermissionFullAccess  PermissionLevel = 4
)

func CreateContext() *ToolContext {
	return &ToolContext{}
}
