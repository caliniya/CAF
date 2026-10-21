package desktop

import "context"

// QQSession 是桌面后端内部的 SDK 适配边界，便于集中测试用假会话验证登录流程。
// 核心和 platform 不依赖这个接口，也不依赖 LagrangeGo 的类型。
type QQSession interface {
	FastLogin() error
	FetchQRCode() ([]byte, error)
	QueryQRCode() (string, error)
	QRCodeLogin() error
	Online() bool
	Account() uint32
	Save() error
	SendGroupText(uint32, string) error
	Close()
}

type QQSessionFactory func(context.Context, QQConfig, string) (QQSession, error)
