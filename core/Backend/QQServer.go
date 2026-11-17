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

	mu       sync.Mutex
	adapters []qqAdapterConfig
	clients  map[string]*qqAdapterRuntime
	core     *Core
	lifeMu   sync.Mutex // 串行执行服务初始化、启动和关闭。
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

// Close 取消全部适配器任务并释放连接，允许重复调用。
// 即使 ctx 已取消也执行清理，并将取消和连接关闭错误返回调用方。
func (q *QQServer) Close(ctx context.Context) error {
	if q == nil {
		return nil
	}
	q.lifeMu.Lock()
	defer q.lifeMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
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
	if ctx != nil && ctx.Err() != nil {
		errs = append(errs, ctx.Err())
	}
	return errors.Join(errs...)
}

type qqAdapterConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Token      string `json:"token"`
	SelfID     string `json:"selfId,omitempty"`
	DetectedQQ string `json:"detectedQQ,omitempty"`
	Port       int    `json:"port"`
}

type qqAdapterRuntime struct {
	client onebot11qq.Extension
	err    string
	cancel context.CancelFunc
}
