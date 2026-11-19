package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
)

type preparedToolCall struct {
	tool       agenttool.Tool
	args       map[string]any
	timeout    time.Duration
	permission agenttool.ToolPermission
}

func prepareToolCall(call api.ToolCall, ctx *agenttool.ToolContext, enabled []api.Tool) (*preparedToolCall, error) {
	tool := agenttool.Tools[call.Name]
	if tool == nil {
		return nil, fmt.Errorf("未找到工具 %q", call.Name)
	}
	meta := tool.Meta()
	if meta == nil {
		return nil, errors.New("工具缺少元信息")
	}
	if !meta.Enabled || !slices.ContainsFunc(enabled, func(item api.Tool) bool { return item.Name == call.Name }) {
		return nil, errToolDisabled
	}
	args := map[string]any{}
	if raw := strings.TrimSpace(call.Arguments); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return nil, fmt.Errorf("参数解析失败：%w", err)
		}
		if args == nil {
			return nil, errors.New("工具参数必须是 JSON 对象")
		}
	}
	timeout, err := meta.TimeoutFor(args)
	if err != nil {
		return nil, err
	}
	permission := agenttool.ToolPermission{Level: meta.Permission}
	if permission.Level == 0 {
		permission.Level = agenttool.PermissionReadOnly
	}
	if aware, ok := tool.(agenttool.PermissionAwareTool); ok {
		permission, err = aware.PermissionFor(ctx, args)
		if err != nil {
			return nil, err
		}
	}
	if permission.Level < agenttool.PermissionReadOnly || permission.Level > agenttool.PermissionFullAccess {
		return nil, errors.New("工具权限声明无效")
	}
	if permission.Reason == "" {
		permission.Reason = fmt.Sprintf("没有执行工具 %q 所需的权限", meta.Name)
	}
	return &preparedToolCall{tool: tool, args: args, timeout: timeout, permission: permission}, nil
}

// invokeTool 的超时从真正开始执行时计算；审批等待不在此函数内。
func invokeTool(prepared *preparedToolCall, ctx *agenttool.ToolContext) (string, error) {
	parent := ctx.Context
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return "", err
	}
	executionContext, cancel := context.WithTimeout(parent, prepared.timeout)
	defer cancel()
	callContext := *ctx
	callContext.Context = executionContext
	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := prepared.tool.Fun(&callContext, prepared.args)
		done <- result{output: output, err: err}
	}()
	select {
	case value := <-done:
		if executionContext.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("执行超时（%s）", prepared.timeout)
		}
		return value.output, value.err
	case <-executionContext.Done():
		if executionContext.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("执行超时（%s）", prepared.timeout)
		}
		return "", executionContext.Err()
	}
}

type toolExecutionRun struct {
	core       *Core
	approvals  *ToolApprovalServer
	session    *chat.ChatTask
	generation *chatSend
	directory  string
	nodeID     string
	context    *agenttool.ToolContext
	enabled    []api.Tool
	strict     bool
	blocked    map[string]api.ToolCall
	seen       map[string]bool
}

func (r *toolExecutionRun) effectiveContext(toolID string, permission agenttool.ToolPermission) (*agenttool.ToolContext, bool) {
	ctx := *r.context
	if r.session.ToolConfig != nil {
		grant, exists := r.session.ToolConfig.AllowedTools[toolID]
		if exists && grant.Scope == agenttool.GrantTool {
			ctx.PermissionLevel = agenttool.PermissionFullAccess
			return &ctx, true
		}
		if exists && grant.Scope == agenttool.GrantWorkspace && grant.WorkspaceID != "" && grant.WorkspaceID == r.session.WorkspaceID && permission.Workspace {
			ctx.PermissionLevel = agenttool.PermissionFullAccess
			ctx.WorkspaceOnly = true
			return &ctx, true
		}
	}
	allowed := ctx.PermissionLevel >= permission.Level && ctx.Check(permission.Level)
	return &ctx, allowed
}

func (r *toolExecutionRun) execute(call *api.ToolCall) (string, error) {
	if call.ID == "" || r.seen[call.ID] {
		return "", errors.New("工具调用 ID 为空或重复")
	}
	r.seen[call.ID] = true
	prepared, err := prepareToolCall(*call, r.context, r.enabled)
	if err != nil {
		return "", err
	}
	if call.Name == agenttool.RequestToolExecutionID {
		if !r.strict {
			return "", errToolDisabled
		}
		ctx := *r.context
		ctx.RequestToolExecution = func(_ context.Context, id string) (string, error) { return r.requestBlocked(call, id) }
		// 申请工具自身只等待审批；被批准的原始工具单独使用它自己的执行超时。
		return prepared.tool.Fun(&ctx, prepared.args)
	}
	ctx, allowed := r.effectiveContext(call.Name, prepared.permission)
	if !allowed {
		if r.strict {
			r.blocked[call.ID] = *call
			return "", fmt.Errorf("%s。严格模式下此调用尚未执行；请调用 RequestToolExecution，参数 call_id 为 %q。批准后将执行原始调用，无需重新调用原工具", prepared.permission.Reason, call.ID)
		}
		ctx, err = r.approve(call, *call, prepared)
		if err != nil {
			return "", err
		}
	}
	r.bindQuestion(ctx, call.ID)
	return invokeTool(prepared, ctx)
}

func (r *toolExecutionRun) bindQuestion(ctx *agenttool.ToolContext, callID string) {
	ctx.AskUser = func(parent context.Context, question agenttool.UserQuestion) (string, error) {
		return r.core.askUser(parent, r.session.ID, r.nodeID, callID, question)
	}
}

func (r *toolExecutionRun) requestBlocked(display *api.ToolCall, id string) (string, error) {
	original, exists := r.blocked[id]
	if !exists {
		return "", errors.New("调用不存在、未被权限拦截，或已经申请处理")
	}
	delete(r.blocked, id)
	prepared, err := prepareToolCall(original, r.context, r.enabled)
	if err != nil {
		return "", err
	}
	ctx, allowed := r.effectiveContext(original.Name, prepared.permission)
	if !allowed {
		ctx, err = r.approve(display, original, prepared)
		if err != nil {
			return "", err
		}
	}
	r.bindQuestion(ctx, display.ID)
	return invokeTool(prepared, ctx)
}

func (r *toolExecutionRun) approve(display *api.ToolCall, original api.ToolCall, prepared *preparedToolCall) (*agenttool.ToolContext, error) {
	view := &api.ToolApproval{
		CallID: original.ID, ToolID: original.Name, Name: agenttool.DisplayName(original.Name), Arguments: original.Arguments,
		Reason: prepared.permission.Reason, WorkspaceID: r.session.WorkspaceID,
		WorkspaceAllowed: prepared.permission.Workspace && r.session.WorkspaceID != "",
	}
	chatServer, err := r.core.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	chatServer.mu.Lock()
	if workspace, err := r.core.workspaceByIDLocked(r.session.WorkspaceID); err == nil && workspace != nil {
		view.WorkspaceName = workspace.Name
	}
	chatServer.mu.Unlock()
	if r.context.PermissionLevel < agenttool.PermissionFullAccess {
		view.HigherPermission = int(agenttool.PermissionFullAccess)
		if prepared.permission.Level <= agenttool.PermissionAutoEdit && view.WorkspaceAllowed && r.context.PermissionLevel < agenttool.PermissionAutoEdit {
			view.HigherPermission = int(agenttool.PermissionAutoEdit)
		}
	}
	display.Approval = view
	decision, err := r.approvals.request(r.context.Context, r.session.ID, r.nodeID, display.ID, view, func(choice string) (*agenttool.ChatToolConfig, error) {
		return r.applyApproval(choice, view)
	})
	if err != nil {
		return nil, err
	}
	if decision == ApprovalReject {
		return nil, errors.New("用户拒绝了本次工具调用，请调整方案，不要重复申请相同操作")
	}
	ctx, allowed := r.effectiveContext(original.Name, prepared.permission)
	if !allowed || decision == ApprovalOnce {
		ctx.PermissionLevel = prepared.permission.Level
		ctx.WorkspaceOnly = prepared.permission.Workspace
	}
	return ctx, nil
}

func (r *toolExecutionRun) applyApproval(choice string, view *api.ToolApproval) (*agenttool.ChatToolConfig, error) {
	server, err := r.core.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if r.context.Context.Err() != nil || server.sending[r.session.ID] != r.generation {
		return nil, errors.New("本次生成已结束，无法批准旧调用")
	}
	if choice == ApprovalReject || choice == ApprovalOnce {
		return r.session.ToolConfig.Clone(), nil
	}
	config := r.session.ToolConfig.Clone()
	if config == nil {
		config = &agenttool.ChatToolConfig{}
	}
	if choice == ApprovalElevate {
		config.Permission = agenttool.PermissionLevel(view.HigherPermission)
	} else {
		if config.AllowedTools == nil {
			config.AllowedTools = make(map[string]agenttool.ToolGrant)
		}
		grant := agenttool.ToolGrant{Scope: agenttool.GrantTool}
		if choice == ApprovalWorkspace {
			grant.Scope, grant.WorkspaceID = agenttool.GrantWorkspace, view.WorkspaceID
		}
		config.AllowedTools[view.ToolID] = grant
	}
	previous := r.session.ToolConfig
	r.session.ToolConfig = config
	r.session.UpdatedAt = time.Now().UTC()
	if err := r.core.saveChatLocked(r.directory, r.session); err != nil {
		r.session.ToolConfig = previous
		return nil, err
	}
	if choice == ApprovalElevate {
		r.context.PermissionLevel = config.Permission
	}
	return config.Clone(), nil
}

func (c *Core) strictToolMode(session *chat.ChatTask) bool {
	if session.ToolConfig != nil && session.ToolConfig.StrictMode != nil {
		return *session.ToolConfig.StrictMode
	}
	return c.Configs != nil && c.Configs.ToolNormalConfig != nil && c.Configs.ToolNormalConfig.StrictMode != nil && *c.Configs.ToolNormalConfig.StrictMode
}

func toolPolicyPrompt(strict bool, ctx *agenttool.ToolContext, config *agenttool.ChatToolConfig) string {
	mode := "权限不足时，宿主会自动提交审批并等待用户选择。"
	if strict {
		mode = "已启用严格模式。权限不足的工具调用不会执行，会返回调用 ID。需要继续时必须主动调用 RequestToolExecution 提交该 ID；批准后宿主会执行保存的原始调用，并由申请工具返回结果，不要再次执行原调用。"
	}
	grants := ""
	if config != nil && len(config.AllowedTools) > 0 {
		body, _ := json.Marshal(config.AllowedTools)
		grants = " 当前对话已授权的工具：" + string(body) + "。工作区授权仅对对应工作区内的操作有效。"
	}
	return fmt.Sprintf("当前工具权限为 %d。%s用户拒绝后请调整方案。%s", ctx.PermissionLevel, mode, grants)
}
