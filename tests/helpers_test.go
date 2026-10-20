package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
	"github.com/caliniya/caf/platforms/desktop"
)

type fakeBackend struct {
	state                                        platform.NativeStatus
	paths                                        platform.Paths
	initCalls, loadCalls, loginCalls, closeCalls int
	initErr, loadErr, loginErr, closeErr         error
}

func (b *fakeBackend) Init(p platform.Paths) error         { b.initCalls++; b.paths = p; return b.initErr }
func (b *fakeBackend) NativeStatus() platform.NativeStatus { return b.state }
func (b *fakeBackend) LoadNative() (platform.NativeStatus, error) {
	b.loadCalls++
	if b.loadErr == nil {
		b.state.Loaded = true
	}
	return b.state, b.loadErr
}
func (b *fakeBackend) BeginLogin(context.Context) error { b.loginCalls++; return b.loginErr }
func (b *fakeBackend) Close() error                     { b.closeCalls++; return b.closeErr }

var _ platform.Backend = (*fakeBackend)(nil)

func testPaths(t *testing.T) platform.Paths {
	t.Helper()
	p, err := desktop.PathsFromExecutable(filepath.Join(t.TempDir(), "CAF.exe"), "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func testRuntime(t *testing.T, b platform.Backend) *core.Runtime {
	t.Helper()
	rt, err := core.NewRuntime(testPaths(t), b)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}
