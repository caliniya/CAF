package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core/platform"
)

func TestDesktopBackendRequiresInitAndDoesNotLoadInConstructor(t *testing.T) {
	b := desktop.New()
	if b.NativeStatus().Loaded {
		t.Fatal("constructor loaded DLL")
	}
	if _, err := b.LoadNative(); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
	if err := b.QQ().BeginLogin(context.Background()); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
}

func TestDesktopPathsCannotChangeAfterInitialization(t *testing.T) {
	p := testPaths(t)
	b := desktop.New()
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if err := b.Init(testPaths(t)); !errors.Is(err, platform.ErrAlreadyInitialized) {
		t.Fatal(err)
	}
	if b.NativeStatus().Loaded {
		t.Fatal("initialization loaded DLL")
	}
}

func TestDesktopLoginExplicitlyReportsPausedWithoutLoading(t *testing.T) {
	p := testPaths(t)
	b := desktop.New()
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if err := b.QQ().BeginLogin(context.Background()); !errors.Is(err, platform.ErrQQIntegrationPaused) {
		t.Fatal(err)
	}
	if b.NativeStatus().Loaded {
		t.Fatal("暂停的登录加载了本地库")
	}
	if _, err := os.Stat(filepath.Join(p.Data, "natives")); !os.IsNotExist(err) {
		t.Fatal("暂停的登录提取了 DLL", err)
	}
}

func TestDesktopNativeLoadingIsPausedOnEveryPlatform(t *testing.T) {
	p := testPaths(t)
	b := desktop.New()
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	before := b.NativeStatus()
	if !before.Paused || before.Loaded || before.Path != "" || before.Exports != 0 {
		t.Fatal("初始原生库状态应为暂停且未加载", before)
	}
	for range 2 {
		state, err := b.LoadNative()
		if !errors.Is(err, platform.ErrQQIntegrationPaused) || state != before || b.NativeStatus() != before {
			t.Fatal("显式调用加载也应保持暂停状态", state, err)
		}
	}
	if _, err := os.Stat(filepath.Join(p.Data, "natives")); !os.IsNotExist(err) {
		t.Fatal("暂停期间不应创建原生库缓存", err)
	}
}

func TestDesktopLoginValidatesContext(t *testing.T) {
	b := desktop.New()
	if err := b.Init(testPaths(t)); err != nil {
		t.Fatal(err)
	}
	if err := b.QQ().BeginLogin(nil); !errors.Is(err, platform.ErrContextRequired) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.QQ().BeginLogin(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDesktopCloseRejectsNewOperations(t *testing.T) {
	b := desktop.New()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Init(testPaths(t)); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := b.LoadNative(); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
	if err := b.QQ().BeginLogin(context.Background()); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
}
