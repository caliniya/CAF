//go:build !windows || !amd64

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/platforms/desktop/native"
)

func TestUnsupportedPlatformIsExplicitAndDoesNotExtract(t *testing.T) {
	data := filepath.Join(t.TempDir(), "not-created")
	loader := native.NewLoader()
	state, err := loader.Load(data)
	if err == nil || state.Supported || state.Loaded {
		t.Fatal("unsupported platform pretended to load", state, err)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatal("unsupported backend wrote a native cache", err)
	}
}
