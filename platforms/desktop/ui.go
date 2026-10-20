package desktop

import (
	"context"
	"os"
	"sync"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/app"
	"github.com/caliniya/caf/core/platform"
)

// App is the UI facade. Only this facade, not the raw backend, is bound to Wails.
type App struct {
	mu      sync.RWMutex
	backend platform.Backend
	service *app.Service
	ctx     context.Context
	err     error
}

// NewApp only stores the injected backend. Wails executes constructors to generate bindings.
func NewApp(backend platform.Backend) *App { return &App{backend: backend} }

func (a *App) Startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.service != nil && a.err == nil {
		return
	}
	a.ctx = ctx
	directories, err := DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
	a.err = err
	if err != nil {
		return
	}
	rt, err := core.NewCore(directories, a.backend)
	a.err = err
	if err != nil {
		return
	}
	a.service, a.err = app.New(rt)
	if a.err == nil {
		a.err = a.service.Init()
	}
}

func (a *App) status() app.Status {
	status := app.Status{Name: "CAF", Version: app.Version}
	if a.service != nil {
		status = a.service.Status()
	}
	if a.err != nil {
		status.LastError = a.err.Error()
	}
	return status
}

func (a *App) GetStatus() app.Status {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status()
}

func (a *App) serviceError() error {
	if a.err != nil {
		return a.err
	}
	if a.service == nil {
		return platform.ErrNotInitialized
	}
	return nil
}

// LoadNativeLibrary only resolves exports; it never creates a QQ session.
func (a *App) LoadNativeLibrary() (app.Status, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if err := a.serviceError(); err != nil {
		return a.status(), err
	}
	return a.service.LoadNative()
}

// BeginLogin delegates to the injected backend. Desktop currently reports
// ErrLoginNotImplemented rather than claiming success or calling unsafe raw ABI.
func (a *App) BeginLogin() (app.Status, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if err := a.serviceError(); err != nil {
		return a.status(), err
	}
	return a.service.BeginLogin(a.ctx)
}

func (a *App) Shutdown(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.service != nil {
		a.err = a.service.Close()
	}
}
