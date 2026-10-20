package app

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/caliniya/CAF-Go/internal/lagrange"
	"github.com/caliniya/CAF-Go/internal/paths"
)

type fakeNative struct {
	state lagrange.Status
	calls int
	data  string
	err   error
}

func (f *fakeNative) Status() lagrange.Status { return f.state }
func (f *fakeNative) Load(data string) (lagrange.Status, error) {
	f.calls++
	f.data = data
	f.state.Loaded = f.err == nil
	return f.state, f.err
}

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	p, err := paths.FromExecutable(filepath.Join(t.TempDir(), "CAF.exe"), "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStartupDoesNotLoadNativeOrLogIn(t *testing.T) {
	loader := &fakeNative{state: lagrange.Status{Supported: true}}
	service, err := New(testPaths(t), loader)
	if err != nil {
		t.Fatal(err)
	}
	status := service.Status()
	if !status.Ready || status.Name != "CAF" || status.Native.Loaded || loader.calls != 0 {
		t.Fatal("unexpected startup state", status)
	}
	status, err = service.LoadNative()
	if err != nil || !status.Native.Loaded || loader.calls != 1 || loader.data != status.DataDir {
		t.Fatal("loader was not injected", status, err)
	}
}

func TestNativeFailureIsReportedAndRetryClearsIt(t *testing.T) {
	failure := errors.New("test native load failure")
	loader := &fakeNative{err: failure}
	service, err := New(testPaths(t), loader)
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.LoadNative()
	if !errors.Is(err, failure) || status.LastError != failure.Error() {
		t.Fatal("failure was lost", status, err)
	}
	loader.err = nil
	status, err = service.LoadNative()
	if err != nil || status.LastError != "" {
		t.Fatal("retry retained stale error", status, err)
	}
}

func TestRequiresLoaderAndWritableDirectory(t *testing.T) {
	if _, err := New(testPaths(t), nil); err == nil {
		t.Fatal("nil loader accepted")
	}
	if _, err := New(paths.Paths{}, &fakeNative{}); err == nil {
		t.Fatal("invalid directory accepted")
	}
}

func TestSerializesConcurrentStatusAndLoad(t *testing.T) {
	service, err := New(testPaths(t), &fakeNative{})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 10 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			service.Status()
			if _, err := service.LoadNative(); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
}
