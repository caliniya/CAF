//go:build windows && amd64

package tests

import (
	"os"
	"os/exec"
	"testing"

	"github.com/caliniya/caf/backend/desktop/native"
)

// 仅在 Windows x64 显式启用诊断时加载库，不调用 Initialize/Start。
func TestLoadsWindowsNativeExportsWithoutLogin(t *testing.T) {
	if os.Getenv("CAF_TEST_LAGRANGE_NATIVE") != "1" {
		t.Skip("QQ 集成已暂停；仅设置 CAF_TEST_LAGRANGE_NATIVE=1 时运行原生加载诊断")
	}
	if os.Getenv("CAF_NATIVE_TEST_CHILD") != "1" {
		// NativeAOT 不能卸载；子进程退出后释放文件锁，再由父进程清理临时目录。
		child := exec.Command(os.Args[0], "-test.run=^TestLoadsWindowsNativeExportsWithoutLogin$", "-test.v")
		child.Env = append(os.Environ(), "CAF_NATIVE_TEST_CHILD=1", "CAF_NATIVE_TEST_DATA="+t.TempDir())
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("native test child: %v\n%s", err, output)
		}
		return
	}
	loader := native.NewLoader()
	state, err := loader.Load(os.Getenv("CAF_NATIVE_TEST_DATA"))
	if err != nil || !state.Loaded || state.Exports != 6 {
		t.Fatal("native loading failed", state, err)
	}
	again, err := loader.Load(t.TempDir())
	if err != nil || again != state {
		t.Fatal("native load should be process-lifetime and idempotent", again, err)
	}
}
