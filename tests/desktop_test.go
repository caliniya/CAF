package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

func TestBindingConstructionHasNoStartupSideEffects(t *testing.T) {
	p := testPaths(t)
	backend := desktop.New()
	engine, err := core.NewCore(p, backend, backend.QQ())
	if err != nil {
		t.Fatal(err)
	}
	status := engine.GetStatus()
	if status.Name != "CAF" || status.Ready || status.DataDir != p.Data || status.Native.Loaded {
		t.Fatal("构造核心不能初始化平台或加载 QQ", status)
	}
	if _, err := os.Stat(p.Data); !os.IsNotExist(err) {
		t.Fatal("构造核心创建了数据目录", err)
	}
	if _, err := engine.LoadNativeLibrary(); err == nil {
		t.Fatal("未启动就允许加载本地库")
	}
}

func TestCoreStartupInitializesDesktopWithoutLoadingNative(t *testing.T) {
	data := filepath.Join(t.TempDir(), "CAF 数据")
	paths, err := desktop.DiscoverPaths(data)
	if err != nil {
		t.Fatal(err)
	}
	backend := desktop.New()
	engine, err := core.NewCore(paths, backend, backend.QQ())
	if err != nil {
		t.Fatal(err)
	}
	engine.Startup(context.Background())
	defer engine.Shutdown(context.Background())
	status := engine.GetStatus()
	if !status.Ready || status.DataDir != data || status.LastError != "" || !status.Native.Paused || status.Native.Loaded {
		t.Fatal("核心启动状态异常", status)
	}
	if info, err := os.Stat(filepath.Join(data, "logs")); err != nil || !info.IsDir() {
		t.Fatal("没有创建日志目录", err)
	}
	if _, err := os.Stat(filepath.Join(data, "natives")); !os.IsNotExist(err) {
		t.Fatal("启动时不应提取本地库", err)
	}
	if err := engine.LoadNative(); !errors.Is(err, platform.ErrNativeRetired) {
		t.Fatal("无界面加载入口也应返回暂停", err)
	}
	status, err = engine.LoadNativeLibrary()
	if !errors.Is(err, platform.ErrNativeRetired) || !status.Ready || !status.Native.Paused || status.Native.Loaded {
		t.Fatal("暂停加载不能影响核心就绪状态", status, err)
	}
	status, err = engine.BeginLogin()
	if !errors.Is(err, platform.ErrQQConfigRequired) || !status.Ready || status.Native.Loaded || status.LastError != err.Error() {
		t.Fatal("暂停登录不能影响核心就绪状态", status, err)
	}
	if _, err := os.Stat(filepath.Join(data, "natives")); !os.IsNotExist(err) {
		t.Fatal("暂停的加载和登录不能偷偷提取本地库", err)
	}
}

func TestCoreStartupReportsInvalidDataDirectory(t *testing.T) {
	data := filepath.Join(t.TempDir(), "不是目录")
	if err := os.WriteFile(data, []byte("保持原样"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := desktop.DiscoverPaths(data)
	if err != nil {
		t.Fatal(err)
	}
	backend := desktop.New()
	engine, err := core.NewCore(paths, backend, backend.QQ())
	if err != nil {
		t.Fatal(err)
	}
	engine.Startup(context.Background())
	status := engine.GetStatus()
	if status.Ready || status.LastError == "" || status.Native.Loaded {
		t.Fatal("启动失败没有保留", status)
	}
	if _, err := engine.LoadNativeLibrary(); err == nil || err.Error() != status.LastError {
		t.Fatal("应返回最初的平台初始化错误", err)
	}
}
