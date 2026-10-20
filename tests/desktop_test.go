package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/platforms/desktop"
)

func TestBindingConstructionHasNoStartupSideEffects(t *testing.T) {
	p := testPaths(t)
	engine, err := core.NewCore(p, desktop.New())
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
	engine, err := core.NewCore(paths, desktop.New())
	if err != nil {
		t.Fatal(err)
	}
	engine.Startup(context.Background())
	defer engine.Shutdown(context.Background())
	status := engine.GetStatus()
	if !status.Ready || status.DataDir != data || status.LastError != "" || status.Native.Loaded {
		t.Fatal("核心启动状态异常", status)
	}
	if info, err := os.Stat(filepath.Join(data, "logs")); err != nil || !info.IsDir() {
		t.Fatal("没有创建日志目录", err)
	}
	if _, err := os.Stat(filepath.Join(data, "natives")); !os.IsNotExist(err) {
		t.Fatal("启动时不应提取本地库", err)
	}
	if _, err := engine.BeginLogin(); err == nil {
		t.Fatal("尚未实现的桌面登录不能返回成功")
	}
	if _, err := os.Stat(filepath.Join(data, "natives")); !os.IsNotExist(err) {
		t.Fatal("未实现的登录不能偷偷提取本地库", err)
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
	engine, err := core.NewCore(paths, desktop.New())
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
