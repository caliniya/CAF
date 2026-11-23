package FileToolInMainTool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	platform "github.com/caliniya/caf/platform"
)

type fileSnapshot struct {
	Exists   bool        `json:"exists"`
	Data     []byte      `json:"data,omitempty"`
	Mode     fs.FileMode `json:"mode,omitempty"`
	Modified time.Time   `json:"modified,omitempty"`
}

// []byte 用 JSON base64 保存，换行、空文件和二进制数据都能原样恢复。
type fileRecovery struct {
	ToolID        string       `json:"toolId"`
	CallID        string       `json:"callId"`
	WorkspaceID   string       `json:"workspaceId,omitempty"`
	Workspace     string       `json:"workspace,omitempty"`
	WorkspaceOnly bool         `json:"workspaceOnly,omitempty"`
	Path          string       `json:"path"`
	Before        fileSnapshot `json:"before"`
	After         fileSnapshot `json:"after"`
}

type fileEditTarget struct{ path, root, relative string }

var fileEditLocks sync.Map

func lockFileEdit(path string) func() {
	lock, _ := fileEditLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

func editFileSystem(ctx *agenttool.ToolContext) (platform.FilePlatform, error) {
	if ctx != nil && ctx.FileSystem != nil {
		return ctx.FileSystem, nil
	}
	return fileSystem()
}

func editingTarget(ctx *agenttool.ToolContext, path string) (fileEditTarget, error) {
	if ctx == nil || !ctx.Check(agenttool.PermissionAutoEdit) {
		return fileEditTarget{}, errors.New("没有修改文件所需的编辑权限；自动编辑需要当前工作区")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return fileEditTarget{}, err
	}
	result := fileEditTarget{path: path}
	if ctx.PermissionLevel == agenttool.PermissionAutoEdit || ctx.WorkspaceOnly {
		result.root, err = filepath.Abs(ctx.WorkSpace)
		if err != nil {
			return fileEditTarget{}, err
		}
		result.relative, err = filepath.Rel(result.root, path)
		if err != nil || !filepath.IsLocal(result.relative) || result.relative == "." {
			return fileEditTarget{}, errors.New("自动编辑权限下只能修改当前工作区内的文件")
		}
	}
	return result, nil
}

func (target fileEditTarget) snapshot(fsys platform.FilePlatform) (fileSnapshot, error) {
	var info fs.FileInfo
	var err error
	if target.root != "" {
		info, err = fsys.StatInRoot(target.root, target.relative)
	} else {
		info, err = fsys.Stat(target.path)
	}
	if errors.Is(err, fs.ErrNotExist) {
		entry, linkErr := target.lstat(fsys)
		if linkErr == nil && entry.Mode()&fs.ModeSymlink != 0 {
			return fileSnapshot{}, errors.New("目标符号链接指向不存在的文件，无法保存可撤回的原始状态")
		}
		if linkErr != nil && !errors.Is(linkErr, fs.ErrNotExist) {
			return fileSnapshot{}, fmt.Errorf("检查目标文件失败：%w", linkErr)
		}
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("检查目标文件失败：%w", err)
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, errors.New("目标路径必须是普通文件")
	}
	var data []byte
	if target.root != "" {
		data, err = fsys.ReadFileInRoot(target.root, target.relative)
	} else {
		data, err = fsys.ReadFile(target.path)
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("读取原始文件失败：%w", err)
	}
	return fileSnapshot{Exists: true, Data: data, Mode: info.Mode().Perm(), Modified: info.ModTime()}, nil
}

func (target fileEditTarget) lstat(fsys platform.FilePlatform) (fs.FileInfo, error) {
	if target.root != "" {
		return fsys.LstatInRoot(target.root, target.relative)
	}
	return fsys.Lstat(target.path)
}

func (target fileEditTarget) write(fsys platform.FilePlatform, data []byte, mode fs.FileMode) error {
	if target.root != "" {
		return fsys.WriteFileInRoot(target.root, target.relative, data, mode)
	}
	return fsys.WriteFile(target.path, data, mode)
}

func (target fileEditTarget) remove(fsys platform.FilePlatform) error {
	// 删除不跟随文件符号链接，也不把目录当作可恢复的文件。
	info, err := target.lstat(fsys)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("删除目标必须是普通文件，不能是文件夹或符号链接")
	}
	if target.root != "" {
		return fsys.DeleteFileInRoot(target.root, target.relative)
	}
	return fsys.DeleteFile(target.path)
}

func rollbackFileEdit(target fileEditTarget, fsys platform.FilePlatform, before fileSnapshot, cause error) error {
	if err := target.restore(fsys, before); err != nil {
		return errors.Join(cause, fmt.Errorf("恢复原始文件失败：%w", err))
	}
	return cause
}

func (target fileEditTarget) restore(fsys platform.FilePlatform, snapshot fileSnapshot) error {
	if !snapshot.Exists {
		// 不递归删除：外部把文件换成非空目录时返回错误，保留其内容。
		if err := target.remove(fsys); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if target.root != "" {
		return fsys.RestoreFileInRoot(target.root, target.relative, snapshot.Data, snapshot.Mode, snapshot.Modified)
	}
	return fsys.RestoreFile(target.path, snapshot.Data, snapshot.Mode, snapshot.Modified)
}

func saveFileRecovery(ctx *agenttool.ToolContext, fsys platform.FilePlatform, toolID string, target fileEditTarget, before, after fileSnapshot) error {
	if ctx.RecoveryFile == "" || !filepath.IsAbs(ctx.RecoveryFile) {
		return errors.New("宿主未提供恢复文件，已取消文件修改")
	}
	if ctx.Context != nil && ctx.Context.Err() != nil {
		return ctx.Context.Err()
	}
	data, err := json.Marshal(fileRecovery{ToolID: toolID, CallID: ctx.CallID, WorkspaceID: ctx.WorkspaceID, Workspace: ctx.WorkSpace, WorkspaceOnly: target.root != "", Path: target.path, Before: before, After: after})
	if err != nil {
		return err
	}
	directory := filepath.Dir(ctx.RecoveryFile)
	if err := fsys.CreateDirectory(directory, 0o700); err != nil {
		return fmt.Errorf("创建工作区恢复缓存失败：%w", err)
	}
	name := filepath.Base(ctx.RecoveryFile)
	temporary := name + ".tmp"
	if err := fsys.WriteFileInRoot(directory, temporary, data, 0o600); err != nil {
		_ = fsys.DeleteFileInRoot(directory, temporary)
		return fmt.Errorf("保存文件恢复数据失败：%w", err)
	}
	if err := fsys.MoveFile(filepath.Join(directory, temporary), ctx.RecoveryFile); err != nil {
		_ = fsys.DeleteFileInRoot(directory, temporary)
		return fmt.Errorf("提交文件恢复数据失败：%w", err)
	}
	return nil
}

func restoreFileRecovery(ctx *agenttool.ToolContext, toolID, recoveryPath string, reapply bool) error {
	if ctx == nil || !ctx.Check(agenttool.PermissionAutoEdit) {
		return errors.New("没有恢复文件所需的编辑权限")
	}
	if !filepath.IsAbs(recoveryPath) {
		return errors.New("恢复文件路径必须为绝对路径")
	}
	if ctx.Context != nil && ctx.Context.Err() != nil {
		return ctx.Context.Err()
	}
	fsys, err := editFileSystem(ctx)
	if err != nil {
		return err
	}
	data, err := fsys.ReadFileInRoot(filepath.Dir(recoveryPath), filepath.Base(recoveryPath))
	if err != nil {
		return fmt.Errorf("读取文件恢复数据失败：%w", err)
	}
	var record fileRecovery
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("文件恢复数据损坏：%w", err)
	}
	if record.ToolID != toolID || record.CallID != ctx.CallID || record.WorkspaceID != ctx.WorkspaceID || record.Workspace != ctx.WorkSpace || !filepath.IsAbs(record.Path) {
		return errors.New("恢复文件与当前工具调用或工作区不一致")
	}
	if toolID == "WriteFile" && !record.After.Exists || toolID == "DeleteFile" && (!record.Before.Exists || record.After.Exists) {
		return errors.New("文件恢复数据中的操作状态无效")
	}
	restoreContext := *ctx
	// 自动编辑和单工具工作区授权在恢复时仍使用原来的目录约束。
	restoreContext.WorkspaceOnly = restoreContext.WorkspaceOnly || record.WorkspaceOnly
	target, err := editingTarget(&restoreContext, record.Path)
	if err != nil {
		return err
	}
	unlock := lockFileEdit(target.path)
	defer unlock()
	snapshot := record.Before
	if reapply {
		snapshot = record.After
	}
	for _, saved := range []fileSnapshot{record.Before, record.After} {
		if saved.Exists && (saved.Mode != saved.Mode.Perm() || saved.Modified.IsZero()) ||
			!saved.Exists && (len(saved.Data) != 0 || saved.Mode != 0 || !saved.Modified.IsZero()) {
			return errors.New("文件恢复数据中的属性无效")
		}
	}
	return target.restore(fsys, snapshot)
}
