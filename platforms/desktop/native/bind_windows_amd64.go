//go:build windows && amd64

package native

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/jamesits/goinvoke"
)

// These are raw addresses, NOT typed wrappers or a complete login API.
type nativeExports struct {
	Initialize           goinvoke.FunctionPointer
	Start                goinvoke.FunctionPointer
	Stop                 goinvoke.FunctionPointer
	FreeMemory           goinvoke.FunctionPointer
	GetQrCodeEvent       goinvoke.FunctionPointer
	GetOnlineEvent       goinvoke.FunctionPointer
	CreateMessageBuilder goinvoke.FunctionPointer
	AddText              goinvoke.FunctionPointer
	SendGroupMessage     goinvoke.FunctionPointer
}

var processLibrary struct {
	sync.Mutex
	path   string
	loaded *nativeExports
	pinned []*nativeExports
}

func bindNative(path string) (string, int, error) {
	processLibrary.Lock()
	defer processLibrary.Unlock()
	if processLibrary.loaded != nil {
		return processLibrary.path, 6, nil
	}
	if !filepath.IsAbs(path) {
		return "", 0, fmt.Errorf("native library path must be absolute")
	}
	exports := &nativeExports{}
	err := goinvoke.Unmarshal(path, exports)

	processLibrary.pinned = append(processLibrary.pinned, exports)
	if err != nil {
		return "", 0, fmt.Errorf("load Lagrange native exports: %w", err)
	}
	for _, proc := range []goinvoke.FunctionPointer{
		exports.Initialize, exports.Start, exports.Stop, exports.FreeMemory,
		exports.GetQrCodeEvent, exports.GetOnlineEvent,
	} {
		if proc == nil {
			return "", 0, fmt.Errorf("goinvoke left a required native export unresolved")
		}
	}
	processLibrary.loaded, processLibrary.path = exports, path
	return path, 6, nil
}
