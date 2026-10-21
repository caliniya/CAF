package platform

import "context"

// QQPlatform 定义 QQ 相关的平台能力，与目录、本地库等基础能力分开。
// 这里只使用普通 Go 类型，不把具体 SDK 的客户端或凭据暴露给核心和页面。
type QQPlatform interface {
	// BeginLogin 发起登录流程；返回 nil 只代表请求受理，不代表已经上线。
	BeginLogin(context.Context) error
}

// QQStatus 只包含可显示的运行状态；不包含签名地址、令牌、设备信息或登录凭据。
// QRCode 是短期有效的本机登录二维码，仅供本地界面显示，不应记录到日志或发送到群。
type QQStatus struct {
	Provider  string `json:"provider"`
	Phase     string `json:"phase"`
	Stage     string `json:"stage,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	QRCode    string `json:"qrCode,omitempty"`
	Account   uint32 `json:"account,omitempty"`
	Error     string `json:"error,omitempty"`
}

// 以下是独立的可选能力，旧的最小 QQ 实现仍可只提供 BeginLogin。
type QQStatusProvider interface{ QQStatus() QQStatus }
type QQLoginControl interface{ CancelLogin() error }
type QQMessenger interface {
	SendGroupText(context.Context, uint32, string) error
}
