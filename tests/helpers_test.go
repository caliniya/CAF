package tests

import (
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/platform"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
)

func testPaths(t *testing.T) platform.Paths {
	t.Helper()
	p, err := desktopBackend.PathsFromExecutable(filepath.Join(t.TempDir(), "CAF.exe"), "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
