package platform

import "io/fs"

//文件系统抽象(这真的需要吗?)
//要不做一个吧
type FilePlatform interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MoveFile(source, destination string) error
	RenameFile(name, newName string) error
	CreateDirectory(name string, perm fs.FileMode) error
	CreateFile(name string, perm fs.FileMode) error
	DeleteFile(name string) error
}
