package Backend

import (
	"context"
	"errors"
	"fmt"
	"sync"

	onebot11qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
)

const QQServerName = "qqServer"

// QQServer 集中管理 QQ 监听配置、适配器配置和运行状态。
// 使用后不得整体复制，内部包含保护 adapters 和 clients 的互斥锁。
type QQServer struct {
	// ListenHost 指定 QQ 服务的监听主机，空值使用 0.0.0.0。
	// 在使用前设置；测试可设为 127.0.0.1，将端口探测和监听限制为本机。
	ListenHost string

	mu        sync.Mutex
	adapters  []qqAdapterConfig
	clients   map[string]*qqAdapterRuntime
	consumers int           // mu 保护；包括已经移出 clients、仍在退出的事件和对话任务。
	draining  chan struct{} // 非空时禁止新启动；最后一个消费者退出时关闭。
	core      *Core
	lifeMu    sync.Mutex // 串行切换服务状态；等待任务退出时不持有。
	chatMu    sync.Mutex
	chats     map[string]*qqChatRuntime
	memoryMu  sync.Mutex
	memories  map[string]*qqPersonalMemory
}

var _ Server = (*QQServer)(nil)

// NewQQServer 创建绑定宿主的独立实例，调用方通过 Servers.Register 注册。
func NewQQServer(core *Core) *QQServer {
	return &QQServer{core: core}
}

// Init 加载适配器配置，不启动监听；重复初始化保留现有运行状态。
func (q *QQServer) Init(ctx context.Context) error {
	if q == nil {
		return errors.New("QQ 服务尚未创建")
	}
	q.lifeMu.Lock()
	defer q.lifeMu.Unlock()
	return q.init(ctx)
}

func (q *QQServer) init(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.core == nil {
		return errors.New("QQ 服务尚未绑定 Core，请使用 NewQQServer 创建")
	}
	registered, err := q.core.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		return err
	}
	if registered != q {
		return errors.New("QQ 服务尚未注册到宿主")
	}
	return q.core.loadQQAdaptersLocked()
}

// Start 恢复已启用扩展的监听，准备就绪后返回；ctx 取消时关闭本次启动的适配器。
// 未启用和未绑定客户端的适配器不会自动启动，失败原因同时保留在运行状态中。
func (q *QQServer) Start(ctx context.Context) error {
	if q == nil {
		return errors.New("QQ 服务尚未创建")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	q.lifeMu.Lock()
	defer q.lifeMu.Unlock()
	if err := q.init(ctx); err != nil {
		return err
	}
	return q.core.startExtensionClients(ctx)
}

// Close 总是先取消任务并释放所有监听端口；ctx 只限制等待后台任务的时间。
// 等待被取消后，再次 Close 仍可等待同一批任务；全部退出后可以重新启动。
func (q *QQServer) Close(ctx context.Context) error {
	if q == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	q.lifeMu.Lock()
	q.mu.Lock()
	var errs []error
	for id, running := range q.clients {
		if running.cancel != nil {
			running.cancel()
		}
		if running.client != nil {
			if err := running.client.Close(); err != nil {
				errs = append(errs, fmt.Errorf("关闭 QQ 适配器 %s：%w", id, err))
			}
		}
		delete(q.clients, id)
	}
	if q.consumers > 0 && q.draining == nil {
		q.draining = make(chan struct{})
	}
	done := q.draining
	q.mu.Unlock()
	q.lifeMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return errors.Join(append(errs, ctx.Err())...)
}

// 事件消费者退出前已等待指令分发和它创建的对话任务结束。
func (q *QQServer) consumerDone() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.consumers--
	if q.consumers == 0 && q.draining != nil {
		close(q.draining)
		q.draining = nil
	}
}

type qqAdapterConfig struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	SelfID     string         `json:"selfId,omitempty"`
	DetectedQQ string         `json:"detectedQQ,omitempty"`
	Port       int            `json:"port"`
	Options    *QQChatOptions `json:"chatOptions,omitempty"`
}

type qqAdapterRuntime struct {
	client onebot11qq.Extension
	err    string
	cancel context.CancelFunc
}
