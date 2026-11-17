package Backend

import (
	"context"
	"errors"
	"sync"

	chat "github.com/caliniya/caf/core/Chat"
)

const ChatServerName = "chatServer"

// ChatServer 保存对话和工具交互的运行状态，使用后不得整体复制。
// ChatList 由 mu 保护；外部调用方通过 Core 的会话 API 访问会话。
type ChatServer struct {
	ChatList map[string]chat.ChatTask

	mu      sync.Mutex
	sending map[string]*chatSend

	interactionMu       sync.Mutex
	interactionRequests map[string]*pendingQuestion

	core   *Core
	lifeMu sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

type chatSend struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

type pendingQuestion struct {
	chatID  string
	context context.Context
	cancel  context.CancelFunc
	answer  chan string
}

var _ Server = (*ChatServer)(nil)

// NewChatServer 绑定并返回 Core 内的对话服务，注册表与 Core 共用同一实例。
func NewChatServer(core *Core) *ChatServer {
	if core == nil {
		return &ChatServer{}
	}
	core.Chat.mu.Lock()
	core.Chat.core = core
	core.Chat.mu.Unlock()
	return &core.Chat
}

// Init 准备运行状态，不覆盖已有客户端、生成任务或交互请求。
// 会话文件继续由原有 API 按需加载，不在初始化时发起模型请求。
func (s *ChatServer) Init(ctx context.Context) error {
	if s == nil {
		return errors.New("对话服务尚未创建")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.core == nil {
		return errors.New("对话服务尚未绑定 Core，请使用 NewChatServer 创建")
	}
	s.initLocked()
	return nil
}

func (s *ChatServer) initLocked() {
	if s.ChatList == nil {
		s.ChatList = make(map[string]chat.ChatTask)
	}
	if s.sending == nil {
		s.sending = make(map[string]*chatSend)
	}
}

// Start 建立生成与交互的生命周期上下文；重复启动保留当前任务。
// 关闭后可重新启动，但前一轮任务必须已经退出。
func (s *ChatServer) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("对话服务尚未创建")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.core == nil {
		return errors.New("对话服务尚未绑定 Core，请使用 NewChatServer 创建")
	}
	s.initLocked()
	if !s.closed && s.ctx != nil && s.ctx.Err() == nil {
		return nil
	}
	if len(s.sending) > 0 {
		return errors.New("上一轮对话生成尚未结束")
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.closed = false
	return nil
}

// contextLocked 兼容直接创建 Core 后调用会话 API 的宿主，首次使用时按需启动。
// 调用方持有 mu；关闭后必须显式 Start 才能恢复生成。
func (s *ChatServer) contextLocked() (context.Context, error) {
	if s.closed {
		return nil, errors.New("对话服务已关闭")
	}
	if s.ctx == nil {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	return s.ctx, nil
}

// Close 取消生成和用户提问等待，等待对话请求保存结果后退出，再释放客户端缓存。
// 等待超过 ctx 期限会返回错误，取消仍然生效；可重复关闭或稍后继续等待。
func (s *ChatServer) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	done := make([]<-chan struct{}, 0, len(s.sending))
	for _, run := range s.sending {
		run.cancel()
		done = append(done, run.done)
	}
	s.interactionMu.Lock()
	for id, pending := range s.interactionRequests {
		pending.cancel()
		delete(s.interactionRequests, id)
	}
	s.interactionMu.Unlock()
	s.mu.Unlock()
	for _, finished := range done {
		select {
		case <-finished:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	s.ChatList = nil
	s.mu.Unlock()
	return ctx.Err()
}
