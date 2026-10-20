// nativecheck 只检查本地库装载与导出解析，不创建 QQ 会话或发起登录。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
	"github.com/caliniya/caf/platforms/desktop"
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
	if !backend.NativeStatus().Supported {
		return platform.ErrUnsupportedPlatform
	}
	directories, err := desktop.DiscoverPaths(data)
	if err != nil {
		return err
	}
	rt, err := core.NewCore(directories, backend)
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
