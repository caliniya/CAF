package Backend

import (
	"context"
	"sync"

	onebot11qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
)

// QQState 集中保存 QQ 监听配置、适配器配置和运行状态，零值可直接使用。
// 使用后不得整体复制，内部包含保护 adapters 和 clients 的互斥锁。
type QQState struct {
	// ListenHost 指定 QQ 服务的监听主机，空值使用 0.0.0.0。
	// 在使用前设置；测试可设为 127.0.0.1，将端口探测和监听限制为本机。
	ListenHost string

	mu       sync.Mutex
	adapters []qqAdapterConfig
	clients  map[string]*qqAdapterRuntime
}

type qqAdapterConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Token      string `json:"token"`
	SelfID     string `json:"selfId,omitempty"`
	DetectedQQ string `json:"detectedQQ,omitempty"`
	Port       int    `json:"port"`
}

type qqAdapterRuntime struct {
	client onebot11qq.Extension
	err    string
	cancel context.CancelFunc
}
