// Package native 保留桌面后端的原生库加载和绑定，不是核心或平台接口的依赖。
// QQ 集成暂停期间，桌面后端不会调用这里的 Load。
package native

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/caliniya/caf/backend/desktop/natives"
	"github.com/caliniya/caf/core/platform"
)

type Loader struct {
	mu     sync.Mutex
	status platform.NativeStatus
}

func NewLoader() *Loader {
	return &Loader{status: platform.NativeStatus{Supported: runtime.GOOS == "windows" && runtime.GOARCH == "amd64"}}
}

func (l *Loader) Status() platform.NativeStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

func (l *Loader) Load(dataDirectory string) (platform.NativeStatus, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status.Loaded {
		return l.status, nil
	}
	if !l.status.Supported {
		return l.status, fmt.Errorf("%w: %s/%s; desktop native backend currently targets windows/amd64", platform.ErrUnsupportedPlatform, runtime.GOOS, runtime.GOARCH)
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
