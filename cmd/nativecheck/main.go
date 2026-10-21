// nativecheck 检查本地库状态；QQ 集成暂停期间只报告暂停，不提取或加载库。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

func main() {
	data := flag.String("data-dir", os.Getenv("CAF_DATA_DIR"), "data directory (absolute recommended with go run)")
	flag.Parse()
	if err := run(*data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(data string) error {
	backend := desktop.New()
	if backend.NativeStatus().Paused {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"status": "paused", "message": platform.ErrQQIntegrationPaused.Error(),
		})
	}
	if !backend.NativeStatus().Supported {
		return platform.ErrUnsupportedPlatform
	}
	directories, err := desktop.DiscoverPaths(data)
	if err != nil {
		return err
	}
	rt, err := core.NewCore(directories, backend, backend.QQ())
	if err != nil {
		return err
	}
	defer rt.Close()
	if err := rt.Init(); err != nil {
		return err
	}
	if err := rt.LoadNative(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "loaded", "message": "6 exports resolved; no QQ session or login started"})
}
