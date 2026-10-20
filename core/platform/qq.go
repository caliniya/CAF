package platform

import "context"

// QQPlatform 定义 QQ 相关的平台能力，与目录、本地库等基础能力分开。
// 这里接收普通 Go 参数并返回 error；DLL 的索引、状态码和内存封送留在后端。
type QQPlatform interface {
	// BeginLogin 发起登录流程；返回 nil 只代表请求受理，不代表已经上线。
	BeginLogin(context.Context) error
}
