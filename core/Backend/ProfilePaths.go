package Backend

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	platform "github.com/caliniya/caf/platform"
)

// File tools may use external workspaces and their user's internal workspaces. Application
// configuration, stores and other users' workspaces stay private at every permission level.
func (c *Core) checkProfilePath(path string) error {
	if c.profileRoot == "" {
		return nil
	}
	denied := errors.New("文件工具不能访问 CAF 用户数据目录，只能在工作区内访问内部文件")
	if insidePath(c.profileRoot, path) && !insidePath(c.Paths.InternalWorkspaces, path) {
		return denied
	}
	realRoot, err := platform.PhysicalPath(c.profileRoot)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(c.profileRoot, c.Paths.InternalWorkspaces)
	if err != nil {
		return err
	}
	probe := filepath.Clean(path)
	for {
		_, err = os.Lstat(probe)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return err
		}
		probe = parent
	}
	real, err := platform.PhysicalPath(probe)
	if err != nil {
		return err
	}
	suffix, err := filepath.Rel(probe, path)
	if err != nil {
		return err
	}
	real = filepath.Join(real, suffix)
	if insidePath(realRoot, real) && !insidePath(filepath.Join(realRoot, rel), real) {
		return denied
	}
	return nil
}
