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

func TestRuntimeConstructionOnlyStoresPathsAndBackend(t *testing.T) {
	p := testPaths(t)
	b := &fakeBackend{state: platform.NativeStatus{Supported: true}}
	rt, err := core.NewCore(p, b)
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

func TestRuntimeRejectsInvalidConfiguration(t *testing.T) {
	if _, err := core.NewCore(testPaths(t), nil); !errors.Is(err, platform.ErrBackendRequired) {
		t.Fatal(err)
	}
	if _, err := core.NewCore(platform.Paths{}, &fakeBackend{}); err == nil {
		t.Fatal("relative/empty paths accepted")
	}
}

func TestRuntimeRequiresExplicitInitialization(t *testing.T) {
	b := &fakeBackend{}
	rt := testRuntime(t, b)
	if err := rt.LoadNative(); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
	if err := rt.BeginLogin(context.Background()); !errors.Is(err, platform.ErrNotInitialized) {
		t.Fatal(err)
	}
	if b.loadCalls != 0 || b.loginCalls != 0 {
		t.Fatal("uninitialized operations reached backend")
	}
}

func TestRuntimeInjectsAndInitializesBackendOnce(t *testing.T) {
	b := &fakeBackend{state: platform.NativeStatus{Supported: true}}
	rt := testRuntime(t, b)
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
	if err := rt.BeginLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.loadCalls != 1 || b.loginCalls != 1 || !rt.State().Native.Loaded {
		t.Fatal("operations did not use the injected backend")
	}
}

func TestRuntimeInitializationFailureCanBeRetried(t *testing.T) {
	failure := errors.New("init failed")
	b := &fakeBackend{initErr: failure}
	rt := testRuntime(t, b)
	if err := rt.Init(); !errors.Is(err, failure) || rt.State().Ready || rt.State().LastError != failure.Error() {
		t.Fatal("init failure was lost", err)
	}
	b.initErr = nil
	if err := rt.Init(); err != nil || !rt.State().Ready || rt.State().LastError != "" {
		t.Fatal("retry failed", err)
	}
}

func TestRuntimeLoadFailureAndRetry(t *testing.T) {
	failure := errors.New("native failed")
	b := &fakeBackend{loadErr: failure}
	rt := testRuntime(t, b)
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

func TestRuntimeChecksLoginContextAndReportsBackendError(t *testing.T) {
	b := &fakeBackend{loginErr: platform.ErrLoginNotImplemented}
	rt := testRuntime(t, b)
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	if err := rt.BeginLogin(nil); !errors.Is(err, platform.ErrContextRequired) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rt.BeginLogin(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if b.loginCalls != 0 {
		t.Fatal("invalid context reached backend")
	}
	if err := rt.BeginLogin(context.Background()); !errors.Is(err, platform.ErrLoginNotImplemented) || b.loginCalls != 1 {
		t.Fatal(err)
	}
}

func TestRuntimeCloseIsIdempotentAndRejectsFurtherOperations(t *testing.T) {
	b := &fakeBackend{}
	rt := testRuntime(t, b)
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
	for _, operation := range []func() error{rt.Init, rt.LoadNative, func() error { return rt.BeginLogin(context.Background()) }} {
		if err := operation(); !errors.Is(err, platform.ErrClosed) {
			t.Fatal(err)
		}
	}
	if b.loadCalls != 0 || b.loginCalls != 0 {
		t.Fatal("closed operation reached backend")
	}
}

func TestRuntimeRetainsCloseFailureWithoutClosingTwice(t *testing.T) {
	failure := errors.New("close failed")
	b := &fakeBackend{closeErr: failure}
	rt := testRuntime(t, b)
	if err := rt.Close(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := rt.Close(); !errors.Is(err, failure) || b.closeCalls != 1 || !rt.State().Closed {
		t.Fatal(err)
	}
}

func TestRuntimeSerializesConcurrentOperations(t *testing.T) {
	b := &fakeBackend{}
	rt := testRuntime(t, b)
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
			if err := rt.BeginLogin(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
	if b.loadCalls != 12 || b.loginCalls != 12 {
		t.Fatal("lost calls")
	}
}
