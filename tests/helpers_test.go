package Tests

import (
	"path/filepath"
	"testing"

	backend "github.com/caliniya/caf/core/Backend"
	platform "github.com/caliniya/caf/platform"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
)

func testServer[T backend.Server](t *testing.T, core *backend.Core, name string) T {
	t.Helper()
	core.InitServers()
	service, err := core.Servers.Get[T](name)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func testPaths(t *testing.T) platform.Paths {
	t.Helper()
	p, err := desktopBackend.PathsFromExecutable(filepath.Join(t.TempDir(), "CAF.exe"), "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
