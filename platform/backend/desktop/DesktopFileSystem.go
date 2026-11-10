package DesktopBackend

import (
	"context"
	"errors"
	"io/fs"
	"os"

	platform "github.com/caliniya/caf/platform"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// DesktopFileSystem 是 FilePlatform 的桌面实现。
type DesktopFileSystem struct {
	ctx context.Context
}

var _ platform.FilePlatform = (*DesktopFileSystem)(nil)

func (fsys *DesktopFileSystem) SelectDirectory(defaultDirectory string) (string, error) {
	if fsys == nil || fsys.ctx == nil {
		return "", errors.New("文件夹选择器尚未初始化")
	}
	if err := fsys.ctx.Err(); err != nil {
		return "", err
	}
	return runtime.OpenDirectoryDialog(fsys.ctx, runtime.OpenDialogOptions{
		Title:            "选择文件夹",
		DefaultDirectory: defaultDirectory,
	})
}

func (fsys *DesktopFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (fsys *DesktopFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (fsys *DesktopFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fsys *DesktopFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (fsys *DesktopFileSystem) WriteFileInRoot(directory, name string, data []byte, perm fs.FileMode) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.WriteFile(name, data, perm)
}

func (fsys *DesktopFileSystem) MoveFile(source, destination string) error {
	return os.Rename(source, destination)
}

func (fsys *DesktopFileSystem) CreateDirectory(name string, perm fs.FileMode) error {
	return os.MkdirAll(name, perm)
}

func (fsys *DesktopFileSystem) DeleteFile(name string) error {
	return os.Remove(name)
}
