package AgentTool

import (
	"context"
	"errors"
	"path/filepath"

	platform "github.com/caliniya/caf/platform"
)

const ReadImageID = "ReadImage"

// 用于工具的一个简单上下文
type ToolContext struct {
	ReadImage     func(context.Context, string) (string, error)
	WorkSpace     string
	WorkspaceID   string
	WorkspaceName string
	// CallID 来自模型；RecoveryFile 是宿主生成的独立恢复文件路径。
	CallID       string
	RecoveryFile string
	FileSystem   platform.FilePlatform
	// CheckPath lets the host reserve private application storage, also in full-access mode.
	CheckPath func(string) error
	PermissionLevel
	// WorkspaceOnly 在单工具工作区授权下仍保留实际写入的目录约束。
	WorkspaceOnly bool
	// Context 随本次工具执行超时而取消，等待交互的工具必须监听它。
	Context context.Context
	// AskUser 由宿主注入；工具模块通过它等待回答，不依赖桌面事件或后端包。
	AskUser              func(context.Context, UserQuestion) (string, error)
	RequestToolExecution ExecutionRequester
	// MemoryScope 返回目标是否属于当前工作区，同时校验启用状态和作用域。
	MemoryScope func(context.Context, string) (bool, error)
	// CreateMemory 由宿主检查对话启用库和库作用域后持久化，返回记忆 ID。
	CreateMemory func(*ToolContext, string, string, []string) (string, error)
	// 只读记忆回调返回 JSON，避免工具基础包依赖宿主的记忆服务。
	ReadMemory   func(context.Context, string, string) (string, error)
	SearchMemory func(context.Context, string, []string) (string, error)
	ListMemories func(context.Context, string) (string, error)
}

type PermissionLevel int

const (
	PermissionReadOnly    PermissionLevel = 1
	PermissionAutoEdit    PermissionLevel = 2
	PermissionAutoApprove PermissionLevel = 3
	PermissionFullAccess  PermissionLevel = 4
)

func CreateContext(Space string, P PermissionLevel) *ToolContext {
	return &ToolContext{
		WorkSpace:       Space,
		PermissionLevel: P,
	}
}

func (c ToolContext) Check(level PermissionLevel) bool {
	if level < PermissionReadOnly || level > PermissionFullAccess {
		return false
	}
	// 自动编辑只能在选中的工作区中写入；没有工作区时只允许读取。
	if (c.PermissionLevel == PermissionAutoEdit || c.WorkspaceOnly) && c.WorkSpace == "" && level >= PermissionAutoEdit {
		return false
	}
	if c.PermissionLevel == PermissionAutoApprove {
		return true
	}
	if c.PermissionLevel >= level {
		return true
	}
	return false
}

// ResolvePath 将相对路径按当前工作区解析，不改变进程的工作目录。
func (c ToolContext) ResolvePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		if c.WorkSpace == "" {
			return "", errors.New("未选择工作区，请使用绝对路径")
		}
		path = filepath.Join(c.WorkSpace, path)
	}
	path = filepath.Clean(path)
	if c.CheckPath != nil {
		if err := c.CheckPath(path); err != nil {
			return "", err
		}
	}
	return path, nil
}
