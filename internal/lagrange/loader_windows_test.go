//go:build windows && amd64

package lagrange

import (
	"os"
	"os/exec"
	"testing"
)

// This test runs only on Windows x64. It resolves symbols but never calls Initialize/Start.
func TestLoadsWindowsNativeExportsWithoutLogin(t *testing.T) {
	if os.Getenv("CAF_NATIVE_TEST_CHILD") != "1" {
		// Native AOT cannot unload. A child process releases the Windows file lock
		// on exit, before the parent's TempDir cleanup attempts to delete the DLL.
		child := exec.Command(os.Args[0], "-test.run=^TestLoadsWindowsNativeExportsWithoutLogin$", "-test.v")
		child.Env = append(os.Environ(), "CAF_NATIVE_TEST_CHILD=1", "CAF_NATIVE_TEST_DATA="+t.TempDir())
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("native test child: %v\n%s", err, output)
		}
		return
	}
	loader := NewLoader()
	state, err := loader.Load(os.Getenv("CAF_NATIVE_TEST_DATA"))
	if err != nil || !state.Loaded || state.Exports != 6 {
		t.Fatal("native loading failed", state, err)
	}
	again, err := loader.Load(t.TempDir())
	if err != nil || again != state {
		t.Fatal("native load should be process-lifetime and idempotent", again, err)
	}
}
