package main

import (
	"context"
	"errors"
	"os"
	"sync"

	core "github.com/caliniya/caf/internal/app"
	"github.com/caliniya/caf/internal/lagrange"
	"github.com/caliniya/caf/internal/paths"
)

// App is the small Wails binding surface. Business code stays independent of Wails.
type App struct {
	mu      sync.RWMutex
	service *core.Service
	err     error
}

// Keep construction side-effect-free: Wails also constructs App to generate bindings.
func NewApp() *App { return &App{} }

func (a *App) startup(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var directories paths.Paths
	directories, a.err = paths.Discover(os.Getenv("CAF_DATA_DIR"))
	if a.err == nil {
		a.service, a.err = core.New(directories, lagrange.NewLoader())
	}
}

func (a *App) GetStatus() core.Status {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.err != nil {
		return core.Status{Name: "CAF", Version: core.Version, LastError: a.err.Error()}
	}
	if a.service == nil {
		return core.Status{Name: "CAF", Version: core.Version}
	}
	return a.service.Status()
}

// Loading only resolves exports. It deliberately does NOT Initialize or Start QQ login.
func (a *App) LoadNativeLibrary() (core.Status, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.err != nil {
		return core.Status{}, a.err
	}
	if a.service == nil {
		return core.Status{}, errors.New("application has not started")
	}
	return a.service.LoadNative()
}
