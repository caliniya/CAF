package Platform

import "io/fs"

// FilePlatform 提供宿主文件系统操作。
type FilePlatform interface {
	// SelectDirectory 打开文件夹选择器；起始目录为空时使用系统默认位置。
	// 用户取消时返回空字符串和 nil。
	SelectDirectory(defaultDirectory string) (string, error)
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	// WriteFileInRoot 的 name 是相对路径，禁止通过路径或符号链接写入 root 之外。
	WriteFileInRoot(root, name string, data []byte, perm fs.FileMode) error
	MoveFile(source, destination string) error
	CreateDirectory(name string, perm fs.FileMode) error
	DeleteFile(name string) error
}
