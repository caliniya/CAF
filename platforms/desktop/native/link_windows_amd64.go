//go:build windows && amd64

package native

import (
	"fmt"
)

// StartLogin 发起登录，nil 只表示请求被受理，不代表上线。
// index 必须是 Initialize 创建的、尚未启动的有效会话索引。
func StartLogin(index int32) error {
	if index < 0 {
		return fmt.Errorf("会话索引不能小于 0")
	}

	processLibrary.Lock()
	defer processLibrary.Unlock()

	exports := processLibrary.loaded
	if exports == nil || exports.Start == nil {
		return fmt.Errorf("请先加载本地库")
	}

	// 此函数用返回状态码报告结果，不使用 Windows 的 LastError。
	result, _, _ := exports.Start.Call(uintptr(index))

	code := int32(result)
	if code != 0 {
		return fmt.Errorf("发起登录失败，状态码：%d", code)
	}

	return nil
}
