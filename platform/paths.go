package platform

import (
	"fmt"
	"path/filepath"
)

// Paths 只保存解析后的绝对路径，不定义平台接口或运行时对象。
type Paths struct {
	Root       string `json:"root"`
	Data       string `json:"data"`
	Logs       string `json:"logs"`
	ConfigFile string
}

// Validate 只校验路径，不读写文件或创建目录。
func (p *Paths) Validate() error {
	for _, item := range []struct{ name, path string }{
		{"root", p.Root}, {"data", p.Data}, {"logs", p.Logs},
	} {
		if !filepath.IsAbs(item.path) {
			return fmt.Errorf("%s path must be absolute", item.name)
		}
	}
	return nil
}
