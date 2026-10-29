package desktopBackend

import (
	"io/fs"
	"os"

	"github.com/caliniya/caf/platform"
)

//paltform.FilePlatform接口的桌面实现
type DesktopFileSystem struct {
}

var _ platform.FilePlatform = (*DesktopFileSystem)(nil)

func (fsys *DesktopFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fsys *DesktopFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (fsys *DesktopFileSystem) MoveFile(source, destination string) error {
	return os.Rename(source, destination)
}

func (fsys *DesktopFileSystem) RenameFile(name, newName string) error {
	return os.Rename(name, newName)
}

func (fsys *DesktopFileSystem) CreateDirectory(name string, perm fs.FileMode) error {
	return os.MkdirAll(name, perm)
}

func (fsys *DesktopFileSystem) CreateFile(name string, perm fs.FileMode) error {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	return file.Close()
}

func (fsys *DesktopFileSystem) DeleteFile(name string) error {
	return os.Remove(name)
}
