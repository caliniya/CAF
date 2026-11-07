package AgentTool

import (
	"errors"
	"path/filepath"
)

// 用于工具的一个简单上下文
type ToolContext struct {
	WorkSpace string
	PermissionLevel
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
	if c.PermissionLevel == PermissionAutoEdit && c.WorkSpace == "" && level >= PermissionAutoEdit {
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
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if c.WorkSpace == "" {
		return "", errors.New("未选择工作区，请使用绝对路径")
	}
	return filepath.Join(c.WorkSpace, path), nil
}
