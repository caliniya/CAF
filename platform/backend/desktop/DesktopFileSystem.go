package DesktopBackend

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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

func (fsys *DesktopFileSystem) Lstat(name string) (fs.FileInfo, error) {
	return os.Lstat(name)
}

func (fsys *DesktopFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (fsys *DesktopFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fsys *DesktopFileSystem) StatInRoot(directory, name string) (fs.FileInfo, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Stat(name)
}

func (fsys *DesktopFileSystem) LstatInRoot(directory, name string) (fs.FileInfo, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Lstat(name)
}

func (fsys *DesktopFileSystem) ReadFileInRoot(directory, name string) ([]byte, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(name)
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

func (fsys *DesktopFileSystem) DeleteFileInRoot(directory, name string) error {
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return errors.New("删除目标必须是根目录内的相对路径，且不能为根目录本身")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}

func (fsys *DesktopFileSystem) RestoreFile(name string, data []byte, perm fs.FileMode, modified time.Time) error {
	if err := os.WriteFile(name, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Chtimes(name, modified, modified)
}

func (fsys *DesktopFileSystem) RestoreFileInRoot(directory, name string, data []byte, perm fs.FileMode, modified time.Time) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.WriteFile(name, data, perm); err != nil {
		return err
	}
	if err := root.Chmod(name, perm); err != nil {
		return err
	}
	return root.Chtimes(name, modified, modified)
}

func (fsys *DesktopFileSystem) RemoveAllInRoot(directory, name string) error {
	name = filepath.Clean(name)
	if !filepath.IsAbs(directory) || name == "." || !filepath.IsLocal(name) {
		return errors.New("删除目标必须是根目录内的相对路径，且不能为根目录本身")
	}
	root, err := os.OpenRoot(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(name)
}
