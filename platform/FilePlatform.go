package Platform

import (
	"io/fs"
	"time"
)

// FilePlatform 提供宿主文件系统操作。
type FilePlatform interface {
	// SelectDirectory 打开文件夹选择器；起始目录为空时使用系统默认位置。
	// 用户取消时返回空字符串和 nil。
	SelectDirectory(defaultDirectory string) (string, error)
	Stat(name string) (fs.FileInfo, error)
	Lstat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	StatInRoot(root, name string) (fs.FileInfo, error)
	LstatInRoot(root, name string) (fs.FileInfo, error)
	ReadFileInRoot(root, name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	// WriteFileInRoot 的 name 是相对路径，禁止通过路径或符号链接写入 root 之外。
	WriteFileInRoot(root, name string, data []byte, perm fs.FileMode) error
	MoveFile(source, destination string) error
	CreateDirectory(name string, perm fs.FileMode) error
	DeleteFile(name string) error
	// DeleteFileInRoot 只删除单个文件或空目录，不递归删除子项。
	DeleteFileInRoot(root, name string) error
	// RestoreFile 恢复原始字节、权限和修改时间。
	RestoreFile(name string, data []byte, perm fs.FileMode, modified time.Time) error
	RestoreFileInRoot(root, name string, data []byte, perm fs.FileMode, modified time.Time) error
	// RemoveAllInRoot 删除 root 内的相对路径及其子项，禁止越过 root 或删除 root 本身。
	RemoveAllInRoot(root, name string) error
}
