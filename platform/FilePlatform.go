package Platform

import "io/fs"

// 文件系统抽象(这真的需要吗?)
// 要不做一个吧
type FilePlatform interface {
	// SelectDirectory 打开文件夹选择器；起始目录为空时使用系统默认位置。
	// 用户取消时返回空字符串和 nil。
	SelectDirectory(defaultDirectory string) (string, error)
	Stat(name string) (fs.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MoveFile(source, destination string) error
	RenameFile(name, newName string) error
	CreateDirectory(name string, perm fs.FileMode) error
	CreateFile(name string, perm fs.FileMode) error
	DeleteFile(name string) error
}
