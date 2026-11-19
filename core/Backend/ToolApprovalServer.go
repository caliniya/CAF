package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	identity "github.com/caliniya/caf/utils/id"
)

const ToolApprovalServerName = "toolApprovalServer"

const (
	ApprovalReject    = "reject"
	ApprovalOnce      = "once"
	ApprovalWorkspace = "workspace"
	ApprovalTool      = "tool"
	ApprovalElevate   = "elevate"
)

type pendingApproval struct {
	chatID string
	ctx    context.Context
	cancel context.CancelFunc
	view   api.ToolApproval
	apply  func(string) (*agenttool.ChatToolConfig, error)
	answer chan string
}

// ToolApprovalServer 管理正在等待的审批，授权规则保存在各自的对话配置中。
type ToolApprovalServer struct {
	core    *Core
	mu      sync.Mutex
	pending map[string]*pendingApproval
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

var _ Server = (*ToolApprovalServer)(nil)

func NewToolApprovalServer(core *Core) *ToolApprovalServer { return &ToolApprovalServer{core: core} }

func (s *ToolApprovalServer) registered() error {
	if s == nil || s.core == nil {
		return errors.New("工具审批服务尚未绑定宿主")
	}
	registered, err := s.core.Servers.Get[*ToolApprovalServer](ToolApprovalServerName)
	if err != nil {
		return err
	}
	if registered != s {
		return errors.New("工具审批服务尚未注册到宿主")
	}
	return nil
}

func (s *ToolApprovalServer) Init(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := s.registered(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[string]*pendingApproval)
	}
	return nil
}

func (s *ToolApprovalServer) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.Init(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && s.ctx != nil && s.ctx.Err() == nil {
		return nil
	}
	if len(s.pending) != 0 {
		return errors.New("上一轮工具审批尚未结束")
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.closed = false
	return nil
}

func (s *ToolApprovalServer) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	for _, request := range s.pending {
		request.cancel()
	}
	s.mu.Unlock()
	if ctx != nil {
		return ctx.Err()
	}
	return nil
}

// request 在发送事件前注册，等待时间只由生成上下文控制，不消耗工具执行超时。
func (s *ToolApprovalServer) request(ctx context.Context, chatID, nodeID, displayCallID string, view *api.ToolApproval, apply func(string) (*agenttool.ChatToolConfig, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	requestID, err := identity.New(identity.Request)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", errors.New("工具审批服务已关闭")
	}
	if s.ctx == nil {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
	if err := s.ctx.Err(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	if s.pending == nil {
		s.pending = make(map[string]*pendingApproval)
	}
	request := &pendingApproval{chatID: chatID, ctx: ctx, cancel: cancel, view: *view, apply: apply, answer: make(chan string, 1)}
	s.pending[requestID] = request
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, requestID)
		s.mu.Unlock()
	}()
	body, _ := json.Marshal(view)
	EmitJSON(EventChatTool, map[string]string{
		"chatId": chatID, "nodeId": nodeID, "callId": displayCallID,
		"event": "update", "requestId": requestID, "approval": string(body),
	})
	select {
	case choice := <-request.answer:
		if err := ctx.Err(); err != nil {
			view.Decision = "canceled"
			return "", err
		}
		view.Decision = choice
		return choice, nil
	case <-ctx.Done():
		view.Decision = "canceled"
		return "", ctx.Err()
	}
}

// SubmitToolApproval 每个请求只能处理一次；保存授权失败时保留请求供用户重试。
func (c *Core) SubmitToolApproval(chatID, requestID, choice string) (*agenttool.ChatToolConfig, error) {
	server, err := c.Servers.Get[*ToolApprovalServer](ToolApprovalServerName)
	if err != nil {
		return nil, err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	request := server.pending[requestID]
	if request == nil || request.chatID != chatID || request.ctx.Err() != nil || server.closed {
		return nil, errors.New("审批已结束或不属于当前对话")
	}
	switch choice {
	case ApprovalReject, ApprovalOnce, ApprovalTool:
	case ApprovalWorkspace:
		if !request.view.WorkspaceAllowed || request.view.WorkspaceID == "" {
			return nil, errors.New("此调用无法授权为工作区内使用")
		}
	case ApprovalElevate:
		if request.view.HigherPermission == 0 {
			return nil, errors.New("当前权限已经足够，无需提升")
		}
	default:
		return nil, errors.New("无效的审批选项")
	}
	config, err := request.apply(choice)
	if err != nil {
		return nil, err
	}
	delete(server.pending, requestID)
	request.answer <- choice
	return config, nil
}
