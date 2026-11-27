package Backend

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	identity "github.com/caliniya/caf/utils/id"
)

func (c *Core) toolRecoveryPath(workspaceID, recoveryID string) (string, error) {
	root, err := c.internalWorkspacesRoot()
	if err != nil {
		return "", err
	}
	if !identity.Valid(recoveryID, identity.Recovery) {
		return "", errors.New("工具恢复 ID 无效")
	}
	folder := workspaceID
	if folder == "" {
		// 旧的无工作区调用可能仍需撤销；新调用始终有实际工作区 ID。
		folder = "unscoped"
	} else if !identity.Valid(folder, identity.Workspace) {
		return "", errors.New("工具恢复工作区 ID 无效")
	}
	return filepath.Join(root, "WorkspaceCache", folder, recoveryID+".json"), nil
}

func (r *toolExecutionRun) invoke(prepared *preparedToolCall, ctx *agenttool.ToolContext, original api.ToolCall) (string, error) {
	ctx.CallID = original.ID
	if original.Name == agenttool.ReadImageID && r.vision != nil {
		before := len(r.vision.pending)
		ctx.ReadImage = func(parent context.Context, source string) (string, error) { return r.vision.read(parent, ctx, source) }
		output, err := invokeTool(prepared, ctx)
		if err != nil {
			r.vision.pending = r.vision.pending[:before]
		}
		return output, err
	}
	if _, reversible := prepared.tool.(agenttool.ReversibleTool); !reversible {
		return invokeTool(prepared, ctx)
	}
	server, err := r.core.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return "", err
	}
	server.toolMu.Lock()
	defer server.toolMu.Unlock()
	id, err := identity.New(identity.Recovery)
	if err != nil {
		return "", err
	}
	ctx.RecoveryFile, err = r.core.toolRecoveryPath(ctx.WorkspaceID, id)
	if err != nil {
		return "", err
	}
	operation := agenttool.ToolOperation{ID: id, ToolID: original.Name, CallID: original.ID, WorkspaceID: ctx.WorkspaceID, Workspace: ctx.WorkSpace, Pending: true}
	// 先保存操作关联；如果取消或宿主意外退出，已有恢复文件仍能找到。
	reply := r.session.Tree.FindNode(r.nodeID)
	if reply == nil {
		return "", errors.New("工具恢复所属的对话节点不存在")
	}
	server.mu.Lock()
	position := len(reply.ToolOperations)
	reply.ToolOperations = append(reply.ToolOperations, operation)
	err = r.core.saveExecutionChatLocked(r.directory, r.session, r.options)
	if err != nil {
		reply.ToolOperations = reply.ToolOperations[:position]
		// 会话文件可能已写入而索引提交失败，尽量移除未执行的关联。
		err = errors.Join(err, r.core.saveExecutionChatLocked(r.directory, r.session, r.options))
	}
	server.mu.Unlock()
	if err != nil {
		return "", fmt.Errorf("保存工具恢复关联失败：%w", err)
	}

	output, callErr := invokeTool(prepared, ctx)
	var changes []agenttool.ToolChange
	var discarded []*agenttool.ToolOperation
	_, cacheErr := r.core.FS.Stat(ctx.RecoveryFile)
	if callErr == nil && cacheErr != nil {
		callErr = fmt.Errorf("工具执行后未找到有效恢复文件：%w", cacheErr)
	}
	if callErr == nil {
		changes, callErr = prepared.tool.(agenttool.ReversibleTool).Changes(ctx, ctx.RecoveryFile)
	}
	if callErr == nil && ctx.Context != nil {
		callErr = ctx.Context.Err()
	}
	if callErr == nil {
		server.mu.Lock()
		path, pathErr := r.session.Tree.CurrentPath()
		callErr = pathErr
		if callErr == nil {
			// 撤销后产生新的修改时，原有重做记录保留展示，但不能覆盖新修改。
			for _, node := range path {
				for i := range node.ToolOperations {
					item := &node.ToolOperations[i]
					if item.State == agenttool.ToolOperationUndone {
						discarded = append(discarded, item)
						item.State = agenttool.ToolOperationDiscarded
					}
				}
			}
			reply.ToolOperations[position].Pending = false
			reply.ToolOperations[position].Changes = changes
			callErr = r.core.saveExecutionChatLocked(r.directory, r.session, r.options)
		}
		server.mu.Unlock()
		if callErr == nil {
			return output, nil
		}
	}

	// 取消和错误也要等实际执行结束再恢复，恢复不使用已经取消的上下文。
	if cacheErr == nil {
		restoreCtx := *ctx
		restoreCtx.Context = context.Background()
		if err := prepared.tool.(agenttool.ReversibleTool).Restore(&restoreCtx, ctx.RecoveryFile); err != nil {
			return output, errors.Join(callErr, err)
		}
	} else if !errors.Is(cacheErr, fs.ErrNotExist) {
		return output, errors.Join(callErr, cacheErr)
	}
	server.mu.Lock()
	for _, item := range discarded {
		item.State = agenttool.ToolOperationUndone
	}
	reply.ToolOperations = reply.ToolOperations[:position]
	saveErr := r.core.saveExecutionChatLocked(r.directory, r.session, r.options)
	server.mu.Unlock()
	if saveErr == nil && cacheErr == nil {
		_ = r.core.FS.DeleteFile(ctx.RecoveryFile)
	}
	return output, errors.Join(callErr, saveErr)
}

func pathToolOperations(path []*chatstruct.ChatNode) []agenttool.ToolOperation {
	var result []agenttool.ToolOperation
	for _, node := range path {
		result = append(result, node.ToolOperations...)
	}
	return result
}

func (c *Core) saveChatWithToolTransition(directory string, session *chat.ChatTask) error {
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	server.toolMu.Lock()
	defer server.toolMu.Unlock()
	server.mu.Lock()
	defer server.mu.Unlock()
	stored, err := c.readChatLocked(directory, session.ID)
	if err != nil {
		return err
	}
	before, pathErr := stored.Tree.CurrentPath()
	after, nextErr := session.Tree.CurrentPath()
	if err := errors.Join(pathErr, nextErr); err != nil {
		return err
	}
	if err := c.transitionToolPaths(before, after); err != nil {
		return err
	}
	if err := c.saveChatLocked(directory, session); err != nil {
		return errors.Join(err, c.transitionToolPaths(after, before), c.saveChatLocked(directory, stored))
	}
	return nil
}

// 调用方持有 ChatServer.mu。每次恢复使用保存的结果，不重新执行模型参数。
func (c *Core) restoreToolOperation(operation agenttool.ToolOperation, reapply bool) error {
	tool, ok := agenttool.Tools[operation.ToolID].(agenttool.ReversibleTool)
	if !ok {
		return fmt.Errorf("可撤回工具 %q 未注册", operation.ToolID)
	}
	path, err := c.toolRecoveryPath(operation.WorkspaceID, operation.ID)
	if err != nil {
		return err
	}
	if _, err := c.FS.Stat(path); errors.Is(err, fs.ErrNotExist) {
		if operation.Pending {
			// 接口约定先保存副本再改文件；尚未保存副本的调用没有变更。
			return nil
		}
		return fmt.Errorf("工具调用 %s 的恢复文件缺失：%w", operation.CallID, err)
	} else if err != nil {
		return err
	}
	if operation.Workspace != "" && !filepath.IsAbs(operation.Workspace) {
		return errors.New("恢复调用的工作区路径无效")
	}
	ctx := agenttool.CreateContext(operation.Workspace, agenttool.PermissionFullAccess)
	if c.profileRoot != "" {
		ctx.CheckPath = c.checkProfilePath
	}
	ctx.CallID, ctx.WorkspaceID, ctx.FileSystem = operation.CallID, operation.WorkspaceID, c.FS
	ctx.Context = context.Background()
	if reapply {
		err = tool.Reapply(ctx, path)
	} else {
		err = tool.Restore(ctx, path)
	}
	if err != nil {
		return fmt.Errorf("恢复工具调用 %s 失败：%w", operation.CallID, err)
	}
	return nil
}

// 公共操作保留，离开分支逆序撤回，进入分支正序重放。
// 中途失败时尽量退回原分支，失败信息完整返回，不保存新的 Choice。
func (c *Core) transitionToolPaths(before, after []*chatstruct.ChatNode) error {
	return c.transitionToolOperations(pathToolOperations(before), pathToolOperations(after))
}

func (c *Core) transitionToolOperations(old, next []agenttool.ToolOperation) error {
	shared := 0
	for shared < len(old) && shared < len(next) && old[shared].ID == next[shared].ID && old[shared].State == next[shared].State {
		shared++
	}
	if shared == len(old) && shared == len(next) {
		return nil
	}
	// 后续分支的 Before 可能包含已撤销祖先的结果，必须回退到它之前的底稿。
	// 只保留连续已应用的公共前缀，未应用的操作也参与逆序还原底稿。
	for i := 0; i < shared; i++ {
		if old[i].State != "" {
			shared = i
			break
		}
	}
	var applied []agenttool.ToolOperation
	rollback := func() error {
		var errs []error
		for i := len(applied) - 1; i >= 0; i-- {
			errs = append(errs, c.restoreToolOperation(applied[i], false))
		}
		for i := len(old) - 1; i >= shared; i-- {
			errs = append(errs, c.restoreToolOperation(old[i], false))
		}
		for _, operation := range old[shared:] {
			if operation.State == "" {
				errs = append(errs, c.restoreToolOperation(operation, true))
			}
		}
		return errors.Join(errs...)
	}
	for i := len(old) - 1; i >= shared; i-- {
		if err := c.restoreToolOperation(old[i], false); err != nil {
			return errors.Join(err, rollback())
		}
	}
	for _, operation := range next[shared:] {
		if operation.State == agenttool.ToolOperationUndone || operation.State == agenttool.ToolOperationDiscarded {
			continue
		}
		applied = append(applied, operation)
		if err := c.restoreToolOperation(operation, true); err != nil {
			return errors.Join(err, rollback())
		}
	}
	return nil
}
