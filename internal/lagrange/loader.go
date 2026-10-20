// Package lagrange currently loads the native library; a full QQ session API is future work.
package lagrange

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/caliniya/CAF-Go/natives"
)

type Status struct {
	Supported bool   `json:"supported"`
	Loaded    bool   `json:"loaded"`
	Path      string `json:"path,omitempty"`
	Exports   int    `json:"exports"`
}

type Loader struct {
	mu     sync.Mutex
	status Status
}

func NewLoader() *Loader {
	return &Loader{status: Status{Supported: runtime.GOOS == "windows" && runtime.GOARCH == "amd64"}}
}

func (l *Loader) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

func (l *Loader) Load(dataDirectory string) (Status, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status.Loaded {
		return l.status, nil
	}
	if !l.status.Supported {
		return l.status, fmt.Errorf("native loading is not implemented for %s/%s; this starter targets windows/amd64", runtime.GOOS, runtime.GOARCH)
	}
	path, err := natives.Extract(dataDirectory)
	if err != nil {
		return l.status, err
	}
	actualPath, count, err := bindNative(path)
	if err != nil {
		return l.status, err
	}
	l.status.Loaded, l.status.Path, l.status.Exports = true, actualPath, count
	return l.status, nil
}
