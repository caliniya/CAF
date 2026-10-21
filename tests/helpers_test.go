package tests

import (
	"path/filepath"
	"testing"

	desktopBackend "github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core/platform"
)

func testPaths(t *testing.T) platform.Paths {
	t.Helper()
	p, err := desktopBackend.PathsFromExecutable(filepath.Join(t.TempDir(), "CAF.exe"), "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
