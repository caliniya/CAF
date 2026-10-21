// Package core 统一处理业务、状态和生命周期，不选择或导入具体平台实现。
package core

import (
	"context"
	"reflect"
	"sync"

	"github.com/caliniya/caf/core/platform"
)

// Platform 是程序的全局核心入口。main 在启动窗口前赋值，运行期间不再替换。
// 核心内部持有后端接口；这里保存指针，避免复制带互斥锁的 Core。
var Platform *Core

// Core 是唯一的业务协调对象，也直接向 Wails 提供可调用的方法。
type Core struct {
	mu          sync.Mutex
	paths       platform.Paths
	backend     platform.Backend
	qq          platform.QQPlatform
	ctx         context.Context
	cancel      context.CancelFunc
	initialized bool
	closed      bool
	lastError   string
	initErr     error
	closeErr    error
}

// NewCore 显式接收基础后端与 QQ 实现，不猜测两种能力是否来自同一个对象。
// qq 可以为 nil；构造只保存引用，不初始化平台、不创建目录、不加载 DLL。
// 测试可创建独立实例；仅 main 将应用使用的实例赋给全局 Platform。
func NewCore(paths platform.Paths, backend platform.Backend, qq platform.QQPlatform) (*Core, error) {
	if nilImplementation(backend) {
		return nil, platform.ErrBackendRequired
	}
	if err := paths.Validate(); err != nil {
		return nil, err
	}
	if nilImplementation(qq) {
		qq = nil
	}
	return &Core{paths: paths, backend: backend, qq: qq}, nil
}

// nilImplementation 识别普通 nil 与装进接口后的带类型 nil，避免调用时崩溃。
func nilImplementation(implementation any) bool {
	if implementation == nil {
		return true
	}
	value := reflect.ValueOf(implementation)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}

// Paths 返回路径副本；初始化之后不能通过此方法修改运行目录。
func (c *Core) Paths() platform.Paths { return c.paths }

func (c *Core) record(err error) error {
	c.lastError = ""
	if err != nil {
		c.lastError = err.Error()
	}
	return err
}

func (c *Core) readyError() error {
	if c.closed {
		return platform.ErrClosed
	}
	if !c.initialized {
		if c.initErr != nil {
			return c.initErr
		}
		return platform.ErrNotInitialized
	}
	return nil
}

func (c *Core) initLocked() error {
	if c.closed {
		return c.record(platform.ErrClosed)
	}
	if c.initialized {
		return nil
	}
	if c.backend == nil {
		c.initErr = platform.ErrBackendRequired
	} else {
		c.initErr = c.backend.Init(c.paths)
	}
	c.initialized = c.initErr == nil
	if c.initialized && c.ctx == nil {
		// 命令行可直接 Init；窗口运行则使用 Startup 提供的生命周期上下文。
		c.ctx, c.cancel = context.WithCancel(context.Background())
	}
	return c.record(c.initErr)
}

// Init 显式初始化平台；成功后不重复初始化，失败后允许重试。
func (c *Core) Init() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initLocked()
}

// Startup 接收 Wails 启动通知；业务状态和上下文都由同一个核心保存。
func (c *Core) Startup(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		c.record(platform.ErrClosed)
		return
	}
	if c.initialized {
		return
	}
	if ctx == nil {
		c.initErr = c.record(platform.ErrContextRequired)
		return
	}
	if err := ctx.Err(); err != nil {
		c.initErr = c.record(err)
		return
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	if c.initLocked() != nil {
		c.cancel()
		c.ctx, c.cancel = nil, nil
	}
}

func (c *Core) loadNativeLocked() error {
	if err := c.readyError(); err != nil {
		return c.record(err)
	}
	_, err := c.backend.LoadNative()
	return c.record(err)
}

// LoadNative 暂留旧接口兼容；桌面后端已改用 Go SDK，返回 ErrNativeRetired。
func (c *Core) LoadNative() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadNativeLocked()
}

// LoadNativeLibrary 为页面返回操作后的状态，仍然操作当前这一个核心。
func (c *Core) LoadNativeLibrary() (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.loadNativeLocked()
	return c.statusLocked(), err
}

// BeginLogin 使用启动时保存的上下文，页面无需传递 Go 的 context 对象。
// QQ 能力与基础 Backend 分开；桌面端尚未实现真实登录时必须明确报错。
func (c *Core) BeginLogin() (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.readyError()
	if err == nil {
		switch {
		case c.ctx == nil:
			err = platform.ErrContextRequired
		case c.ctx.Err() != nil:
			err = c.ctx.Err()
		case c.qq == nil:
			err = platform.ErrQQUnavailable
		default:
			err = c.qq.BeginLogin(c.ctx)
		}
	}
	c.record(err)
	return c.statusLocked(), err
}

// Close 先取消核心的任务上下文，再关闭平台；重复调用只返回第一次的结果。
// 桌面后端会取消 QQ 登录任务并关闭 Go SDK 会话。
func (c *Core) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return c.closeErr
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	if c.backend != nil {
		c.closeErr = c.backend.Close()
	}
	return c.record(c.closeErr)
}

// Shutdown 接收 Wails 关闭通知，不创建新实例，也不重复关闭后端。
func (c *Core) Shutdown(_ context.Context) { _ = c.Close() }
