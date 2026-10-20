package platform

import (
	"fmt"
	"path/filepath"
)

// Paths 保存解析后的绝对路径。平台选择默认目录，核心统一保存这些数据。
type Paths struct {
	Root string `json:"root"`
	Data string `json:"data"`
	Logs string `json:"logs"`
}

// Validate 只校验路径，不读写文件；创建目录由后端在初始化时负责。
func (p Paths) Validate() error {
	for _, item := range []struct{ name, path string }{
		{"root", p.Root}, {"data", p.Data}, {"logs", p.Logs},
	} {
		if !filepath.IsAbs(item.path) {
			return fmt.Errorf("%s path must be absolute", item.name)
		}
	}
	return nil
}
