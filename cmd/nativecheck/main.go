// nativecheck 暂留兼容旧命令；LagrangeGo 不需要装载 C# 本地库。
package main

import (
	"encoding/json"
	"flag"
	"os"
)

func main() {
	flag.String("data-dir", "", "兼容旧参数，不创建数据目录")
	flag.Parse()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "not_required", "provider": "LagrangeGo", "message": "QQ 已改用 Go SDK，无需 C# DLL/so"})
}
