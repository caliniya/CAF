// Package desktop implements platform contracts for the desktop application.
package desktop

import (
	"context"
	"os"
	"sync"

	"github.com/caliniya/caf/core/platform"
	"github.com/caliniya/caf/platforms/desktop/native"
)

type DesktopBackend struct {
	mu          sync.Mutex
	paths       platform.Paths
	loader      *native.Loader
	initialized bool
	closed      bool
}

var _ platform.Backend = (*DesktopBackend)(nil)

// New is safe during Wails binding generation: no directories, DLL or QQ session.
func New() *DesktopBackend { return &DesktopBackend{loader: native.NewLoader()} }

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
	return b.loader.Status()
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
		return b.loader.Status(), err
	}
	return b.loader.Load(b.paths.Data)
}

func (b *DesktopBackend) BeginLogin(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.readyError(); err != nil {
		return err
	}
	if ctx == nil {
		return platform.ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Native parameter marshalling/session/event handling is not implemented yet.
	// Never pretend login succeeded or call Initialize/Start with guessed arguments.
	return platform.ErrLoginNotImplemented
}

func (b *DesktopBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	// No sessions/tasks exist yet. NativeAOT mappings remain pinned until process exit.
	return nil
}
