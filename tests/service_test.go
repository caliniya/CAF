package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/caliniya/caf/core/app"
	"github.com/caliniya/caf/core/platform"
)

func TestApplicationServiceUsesSharedRuntime(t *testing.T) {
	b := &fakeBackend{state: platform.NativeStatus{Supported: true}}
	rt := testRuntime(t, b)
	service, err := app.New(rt)
	if err != nil {
		t.Fatal(err)
	}
	if service.Status().Ready || b.initCalls != 0 {
		t.Fatal("service construction initialized backend")
	}
	if err := service.Init(); err != nil {
		t.Fatal(err)
	}
	state := service.Status()
	if !state.Ready || state.Name != "CAF" || state.DataDir != rt.Paths().Data || b.loadCalls != 0 {
		t.Fatal(state)
	}
	state, err = service.LoadNative()
	if err != nil || !state.Native.Loaded || b.loadCalls != 1 {
		t.Fatal(state, err)
	}
	if _, err := service.BeginLogin(context.Background()); err != nil || b.loginCalls != 1 {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil || !service.Status().Closed {
		t.Fatal(err)
	}
}

func TestApplicationServiceReportsRuntimeErrors(t *testing.T) {
	b := &fakeBackend{loadErr: errors.New("test load failure"), loginErr: platform.ErrLoginNotImplemented}
	service, err := app.New(testRuntime(t, b))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Init(); err != nil {
		t.Fatal(err)
	}
	state, err := service.LoadNative()
	if !errors.Is(err, b.loadErr) || state.LastError != err.Error() {
		t.Fatal(state, err)
	}
	b.loadErr = nil
	state, err = service.LoadNative()
	if err != nil || state.LastError != "" {
		t.Fatal(state, err)
	}
	state, err = service.BeginLogin(context.Background())
	if !errors.Is(err, platform.ErrLoginNotImplemented) || state.LastError != err.Error() {
		t.Fatal(state, err)
	}
}

func TestApplicationServiceRequiresRuntime(t *testing.T) {
	if _, err := app.New(nil); err == nil {
		t.Fatal("nil runtime accepted")
	}
}
