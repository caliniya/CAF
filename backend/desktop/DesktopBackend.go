// Package desktop 提供桌面平台实现，不持有或创建另一份核心。
package desktop

import (
	"os"
	"sync"

	"github.com/caliniya/caf/core/platform"
)

type DesktopBackend struct {
	mu          sync.Mutex
	paths       platform.Paths
	qq          *DesktopQQ
	initialized bool
	closed      bool
}

var _ platform.Backend = (*DesktopBackend)(nil)

// New 只创建后端对象，不创建目录、不联网，也不尝试登录。
func New() *DesktopBackend {
	return NewWithQQFactory(NewLagrangeSession)
}

// NewWithQQFactory 允许集中测试注入假会话；应用入口直接使用 New。
func NewWithQQFactory(factory QQSessionFactory) *DesktopBackend {
	if factory == nil {
		factory = NewLagrangeSession
	}
	b := &DesktopBackend{}
	b.qq = &DesktopQQ{backend: b, factory: factory, state: platform.QQStatus{Provider: "LagrangeGo", Phase: "idle"}}
	return b
}

func (b *DesktopBackend) Init(paths platform.Paths) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return platform.ErrClosed
	}
	if err := paths.Validate(); err != nil {
		return err
	}
	if b.initialized {
		if b.paths != paths {
			return platform.ErrAlreadyInitialized
		}
		return nil
	}
	if err := os.MkdirAll(paths.Data, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.Logs, 0700); err != nil {
		return err
	}
	if err := prepareQQDirectory(paths.Data); err != nil {
		return err
	}

	b.paths, b.initialized = paths, true
	return nil
}

func (b *DesktopBackend) NativeStatus() platform.NativeStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nativeStatusLocked()
}

func (b *DesktopBackend) nativeStatusLocked() platform.NativeStatus {
	// 旧接口暂留兼容；QQ 改用 Go SDK，归档 DLL 不进入运行流程。
	return platform.NativeStatus{Paused: true}
}

func (b *DesktopBackend) readyError() error {
	if b.closed {
		return platform.ErrClosed
	}
	if !b.initialized {
		return platform.ErrNotInitialized
	}
	return nil
}

func (b *DesktopBackend) LoadNative() (platform.NativeStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.readyError(); err != nil {
		return b.nativeStatusLocked(), err
	}
	// 即使调用方显式请求，也不提取或加载 DLL；Linux/WSL 无需准备 .so。
	return b.nativeStatusLocked(), platform.ErrNativeRetired
}

func (b *DesktopBackend) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	if b.qq != nil {
		b.qq.close()
	}
	return nil
}
