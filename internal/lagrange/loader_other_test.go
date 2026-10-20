//go:build !windows || !amd64

package lagrange

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsupportedPlatformIsExplicitAndDoesNotExtract(t *testing.T) {
	data := filepath.Join(t.TempDir(), "not-created")
	loader := NewLoader()
	state, err := loader.Load(data)
	if err == nil || state.Supported || state.Loaded {
		t.Fatal("unsupported platform pretended to load", state, err)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatal("unsupported backend wrote a native cache", err)
	}
}
