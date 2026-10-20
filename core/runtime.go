// Package core owns the shared runtime; it never selects a concrete platform.
package core

import (
	"context"
	"sync"

	"github.com/caliniya/caf/core/platform"
)

type State struct {
	Ready     bool
	Closed    bool
	Native    platform.NativeStatus
	LastError string
}

// Runtime owns one backend and one immutable set of paths. All backend operations
// are serialized, including shutdown. Use separate instances for isolated tests.
type Runtime struct {
	mu          sync.Mutex
	paths       platform.Paths
	backend     platform.Backend
	initialized bool
	closed      bool
	lastError   string
	closeErr    error
}

// NewRuntime only validates and stores references; it does not initialize or load anything.
func NewRuntime(paths platform.Paths, backend platform.Backend) (*Runtime, error) {
	if backend == nil {
		return nil, platform.ErrBackendRequired
	}
	if err := paths.Validate(); err != nil {
		return nil, err
	}
	return &Runtime{paths: paths, backend: backend}, nil
}

// Paths returns a value copy, not a mutable reference to runtime configuration.
func (r *Runtime) Paths() platform.Paths { return r.paths }

func (r *Runtime) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{
		Ready: r.initialized && !r.closed, Closed: r.closed,
		Native: r.backend.NativeStatus(), LastError: r.lastError,
	}
}

func (r *Runtime) record(err error) error {
	r.lastError = ""
	if err != nil {
		r.lastError = err.Error()
	}
	return err
}

func (r *Runtime) readyError() error {
	if r.closed {
		return platform.ErrClosed
	}
	if !r.initialized {
		return platform.ErrNotInitialized
	}
	return nil
}

// Init is explicit and retryable on failure. A successful initialization runs only once.
func (r *Runtime) Init() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.record(platform.ErrClosed)
	}
	if r.initialized {
		return nil
	}
	err := r.backend.Init(r.paths)
	r.initialized = err == nil
	return r.record(err)
}

func (r *Runtime) LoadNative() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.readyError(); err != nil {
		return r.record(err)
	}
	_, err := r.backend.LoadNative()
	return r.record(err)
}

func (r *Runtime) BeginLogin(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.readyError(); err != nil {
		return r.record(err)
	}
	if ctx == nil {
		return r.record(platform.ErrContextRequired)
	}
	if err := ctx.Err(); err != nil {
		return r.record(err)
	}
	return r.record(r.backend.BeginLogin(ctx))
}

func (r *Runtime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.closeErr = r.backend.Close()
	return r.record(r.closeErr)
}
