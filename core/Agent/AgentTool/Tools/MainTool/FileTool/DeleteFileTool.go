package FileToolInMainTool

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type DeleteFile struct{ agenttool.ToolMeta }

var _ agenttool.ReversibleTool = (*DeleteFile)(nil)

func (t *DeleteFile) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID: "DeleteFile", Name: "删除文件", Enabled: true,
		Description: "删除指定的普通文件，不删除文件夹或递归删除目录。path 为路径片段数组，相对路径以当前工作区为基准。删除前自动保存恢复副本；无法保存副本时不删除。自动编辑权限只能删除当前工作区中的文件，工作区外需要授权。",
		Permission:  agenttool.PermissionAutoEdit, TimeoutParam: "timeout_seconds", DisplayType: "info", DisplayContent: "arguments.path",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": pathParameter()}, "required": []string{"path"}},
	}
}

func (t *DeleteFile) Meta() *agenttool.ToolMeta { return &t.ToolMeta }

func (t *DeleteFile) PermissionFor(ctx *agenttool.ToolContext, args map[string]any) (agenttool.ToolPermission, error) {
	path, err := resolveFilePath(ctx, args)
	if err != nil {
		return agenttool.ToolPermission{}, err
	}
	if ctx != nil && ctx.WorkSpace != "" {
		workspace, err := filepath.Abs(ctx.WorkSpace)
		if err != nil {
			return agenttool.ToolPermission{}, err
		}
		target, err := filepath.Abs(path)
		if err != nil {
			return agenttool.ToolPermission{}, err
		}
		relative, err := filepath.Rel(workspace, target)
		if err == nil && filepath.IsLocal(relative) {
			return agenttool.ToolPermission{Level: agenttool.PermissionAutoEdit, Workspace: true, Reason: "当前权限不允许删除工作区文件"}, nil
		}
	}
	return agenttool.ToolPermission{Level: agenttool.PermissionFullAccess, Reason: "删除工作区外的文件需要授权"}, nil
}

func (t *DeleteFile) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	path, err := resolveFilePath(ctx, args)
	if err != nil {
		return "", err
	}
	target, err := editingTarget(ctx, path)
	if err != nil {
		return "", err
	}
	fsys, err := editFileSystem(ctx)
	if err != nil {
		return "", err
	}
	unlock := lockFileEdit(target.path)
	defer unlock()
	info, err := target.lstat(fsys)
	if err != nil {
		return "", fmt.Errorf("检查删除目标失败：%w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("删除目标必须是普通文件，不能是文件夹或符号链接")
	}
	before, err := target.snapshot(fsys)
	if err != nil {
		return "", err
	}
	if !before.Exists {
		return "", errors.New("要删除的文件不存在")
	}
	if err := saveFileRecovery(ctx, fsys, t.ID, target, before, fileSnapshot{}); err != nil {
		return "", err
	}
	if ctx.Context != nil && ctx.Context.Err() != nil {
		return "", ctx.Context.Err()
	}
	if err := target.remove(fsys); err != nil {
		return "", rollbackFileEdit(target, fsys, before, fmt.Errorf("删除文件失败：%w", err))
	}
	if ctx.Context != nil && ctx.Context.Err() != nil {
		return "", rollbackFileEdit(target, fsys, before, ctx.Context.Err())
	}
	body, err := json.Marshal(struct {
		Path    string `json:"path"`
		Deleted bool   `json:"deleted"`
	}{target.path, true})
	return string(body), err
}

func (t *DeleteFile) Restore(ctx *agenttool.ToolContext, recoveryPath string) error {
	return restoreFileRecovery(ctx, t.ID, recoveryPath, false)
}
func (t *DeleteFile) Reapply(ctx *agenttool.ToolContext, recoveryPath string) error {
	return restoreFileRecovery(ctx, t.ID, recoveryPath, true)
}
