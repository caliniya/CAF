package tests

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

func TestCoreConstructionOnlyStoresPathsAndBackend(t *testing.T) {
	p := testPaths(t)
	b := &fakeBackend{state: platform.NativeStatus{Supported: true}}
	rt, err := core.NewCore(p, b, b)
	if err != nil {
		t.Fatal(err)
	}
	if rt.State().Ready || b.initCalls != 0 || b.loadCalls != 0 || b.loginCalls != 0 {
		t.Fatal("constructor initialized the backend")
	}
	if _, err := os.Stat(p.Data); !os.IsNotExist(err) {
		t.Fatal("constructor created data", err)
	}
	copy := rt.Paths()
	copy.Data = "changed"
	if rt.Paths() != p {
		t.Fatal("paths were mutable through the getter")
	}
}

func TestCoreRejectsInvalidConfiguration(t *testing.T) {
	if _, err := core.NewCore(testPaths(t), nil, nil); !errors.Is(err, platform.ErrBackendRequired) {
		t.Fatal(err)
	}
	if _, err := core.NewCore(platform.Paths{}, &fakeBackend{}, nil); err == nil {
		t.Fatal("relative/empty paths accepted")
	}
}

func TestCoreRequiresExplicitInitialization(t *testing.T) {
	b := &fakeBackend{}
	rt := testCore(t, b)
	if err := rt.LoadNative(); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
	if _, err := rt.BeginLogin(); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
	if b.loadCalls != 0 || b.loginCalls != 0 {
		t.Fatal("uninitialized operations reached backend")
	}
}

func TestCoreInjectsAndInitializesBackendOnce(t *testing.T) {
	b := &fakeBackend{state: platform.NativeStatus{Supported: true}}
	rt := testCore(t, b)
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	if b.initCalls != 1 || b.paths != rt.Paths() || !rt.State().Ready {
		t.Fatal("incorrect backend initialization")
	}
	if err := rt.LoadNative(); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	if b.loadCalls != 1 || b.loginCalls != 1 || !rt.State().Native.Loaded {
		t.Fatal("operations did not use the injected backend")
	}
}

func TestCoreInitializationFailureCanBeRetried(t *testing.T) {
	failure := errors.New("init failed")
	b := &fakeBackend{initErr: failure}
	rt := testCore(t, b)
	if err := rt.Init(); !errors.Is(err, failure) || rt.State().Ready || rt.State().LastError != failure.Error() {
		t.Fatal("init failure was lost", err)
	}
	b.initErr = nil
	if err := rt.Init(); err != nil || !rt.State().Ready || rt.State().LastError != "" {
		t.Fatal("retry failed", err)
	}
}

func TestCoreLoadFailureAndRetry(t *testing.T) {
	failure := errors.New("native failed")
	b := &fakeBackend{loadErr: failure}
	rt := testCore(t, b)
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	if err := rt.LoadNative(); !errors.Is(err, failure) || rt.State().LastError != failure.Error() {
		t.Fatal(err)
	}
	b.loadErr = nil
	if err := rt.LoadNative(); err != nil || !rt.State().Native.Loaded || rt.State().LastError != "" {
		t.Fatal(err)
	}
}

func TestCoreChecksLoginContextAndReportsBackendError(t *testing.T) {
	b := &fakeBackend{loginErr: platform.ErrLoginNotImplemented}
	rt := testCore(t, b)
	rt.Startup(nil)
	if _, err := rt.BeginLogin(); !errors.Is(err, platform.ErrContextRequired) {
		t.Fatal("没有保留无效启动上下文的错误", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.Startup(ctx)
	cancel()
	if _, err := rt.BeginLogin(); !errors.Is(err, context.Canceled) {
		t.Fatal("未使用核心保存的生命周期上下文", err)
	}
	if b.loginCalls != 0 {
		t.Fatal("无效上下文进入了平台后端")
	}
	// 另建独立实例验证平台返回的真实错误，不伪装登录成功。
	other := testCore(t, b)
	other.Startup(context.Background())
	if _, err := other.BeginLogin(); !errors.Is(err, platform.ErrLoginNotImplemented) || b.loginCalls != 1 {
		t.Fatal(err)
	}
}

func TestCoreCloseIsIdempotentAndRejectsFurtherOperations(t *testing.T) {
	b := &fakeBackend{}
	rt := testCore(t, b)
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Close(); err != nil || b.closeCalls != 1 {
		t.Fatal("closed twice", err)
	}
	if !rt.State().Closed || rt.State().Ready {
		t.Fatal("bad closed state")
	}
	for _, operation := range []func() error{rt.Init, rt.LoadNative, func() error { _, err := rt.BeginLogin(); return err }} {
		if err := operation(); !errors.Is(err, platform.ErrClosed) {
			t.Fatal(err)
		}
	}
	if b.loadCalls != 0 || b.loginCalls != 0 {
		t.Fatal("closed operation reached backend")
	}
}

func TestCoreRetainsCloseFailureWithoutClosingTwice(t *testing.T) {
	failure := errors.New("close failed")
	b := &fakeBackend{closeErr: failure}
	rt := testCore(t, b)
	if err := rt.Close(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := rt.Close(); !errors.Is(err, failure) || b.closeCalls != 1 || !rt.State().Closed {
		t.Fatal(err)
	}
}

func TestCoreSerializesConcurrentOperations(t *testing.T) {
	b := &fakeBackend{}
	rt := testCore(t, b)
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			rt.State()
			rt.Paths()
			if err := rt.LoadNative(); err != nil {
				t.Error(err)
			}
			if _, err := rt.BeginLogin(); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
	if b.loadCalls != 12 || b.loginCalls != 12 {
		t.Fatal("lost calls")
	}
}
