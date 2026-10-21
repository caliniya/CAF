// Package desktop 提供桌面平台实现，不持有或创建另一份核心。
package desktop

import (
	"os"
	"sync"

	"github.com/caliniya/caf/backend/desktop/native"
	"github.com/caliniya/caf/core/platform"
)

type DesktopBackend struct {
	mu          sync.Mutex
	paths       platform.Paths
	loader      *native.Loader
	qq          *DesktopQQ
	initialized bool
	closed      bool
}

var _ platform.Backend = (*DesktopBackend)(nil)

// New 只创建后端对象，不创建目录、不加载 DLL，也不尝试登录。
func New() *DesktopBackend {
	b := &DesktopBackend{loader: native.NewLoader()}
	b.qq = &DesktopQQ{backend: b}
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

	b.paths, b.initialized = paths, true
	return nil
}

func (b *DesktopBackend) NativeStatus() platform.NativeStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nativeStatusLocked()
}

func (b *DesktopBackend) nativeStatusLocked() platform.NativeStatus {
	status := b.loader.Status()
	// 保留加载器、绑定和二进制资源；当前阶段所有桌面系统都暂停 QQ 集成。
	status.Paused = true
	return status
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
	return b.nativeStatusLocked(), platform.ErrQQIntegrationPaused
}

func (b *DesktopBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	// 暂无已创建的 QQ 会话；NativeAOT 映射保留到进程退出，不执行卸载。
	return nil
}
