package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/caliniya/caf/core/platform"
)

func DiscoverPaths(dataOverride string) (platform.Paths, error) {
	executable, err := os.Executable()
	if err != nil {
		return platform.Paths{}, err
	}
	return PathsFromExecutable(executable, dataOverride)
}

// PathsFromExecutable 只解析默认路径，不创建文件。
// 相对覆盖路径始终按可执行文件目录解析，不按进程工作目录解析。
func PathsFromExecutable(executable, dataOverride string) (platform.Paths, error) {
	if !filepath.IsAbs(executable) {
		return platform.Paths{}, errors.New("an absolute executable path is required")
	}
	root := filepath.Dir(filepath.Clean(executable))
	data := dataOverride
	if data == "" {
		data = "data"
	}
	if !filepath.IsAbs(data) {
		data = filepath.Join(root, data)
	}
	data = filepath.Clean(data)
	return platform.Paths{Root: root, Data: data, Logs: filepath.Join(data, "logs")}, nil
}
