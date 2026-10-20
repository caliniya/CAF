// Package platform 定义平台接口与普通 Go 数据，不包含具体平台实现。
package platform

import (
	"errors"
)

var (
	ErrBackendRequired     = errors.New("尚未提供平台后端")
	ErrNotInitialized      = errors.New("核心或平台后端尚未初始化")
	ErrAlreadyInitialized  = errors.New("初始化后不能更改平台目录")
	ErrClosed              = errors.New("核心或平台后端已经关闭")
	ErrContextRequired     = errors.New("必须提供有效的上下文")
	ErrUnsupportedPlatform = errors.New("当前平台尚未实现本地库加载")
	ErrLoginNotImplemented = errors.New("QQ 登录尚未实现；目前只支持加载本地库并解析导出")
	ErrQQUnavailable       = errors.New("当前平台未提供 QQ 接口")
)

type NativeStatus struct {
	Supported bool   `json:"supported"`
	Loaded    bool   `json:"loaded"`
	Path      string `json:"path,omitempty"`
	Exports   int    `json:"exports"`
}

// Backend 只负责基础平台能力；需要 QQ 功能时，后端另外实现 QQPlatform。
// 核心负责生命周期与业务编排，后端不应再次创建核心或卸载 NativeAOT 库。
type Backend interface {
	Init(Paths) error
	NativeStatus() NativeStatus
	LoadNative() (NativeStatus, error)
	Close() error
}
