package Backend

import (
	"sync"

	chat "github.com/caliniya/caf/core/Chat"
	platform "github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	Paths   *platform.Paths
	Configs *Configs
	//APIClients *[]netclient.APIClient
	FS platform.FilePlatform
	// QQAdapterListenHost 指定 QQ 服务的监听主机，空值使用 0.0.0.0。
	// 测试可设为 127.0.0.1，将端口探测和服务监听限制为本机。
	QQAdapterListenHost string

	chatMu              sync.Mutex
	chatSending         map[string]bool
	interactionMu       sync.Mutex
	interactionRequests map[string]*pendingQuestion

	qqMu       sync.Mutex
	qqAdapters []qqAdapterConfig
	qqClients  map[string]*qqAdapterRuntime
	extMu      sync.Mutex

	ChatList map[string]chat.ChatTask
}

func (c *Core) GetPaths() *platform.Paths {
	return c.Paths // Paths 有 json tag,可以直接序列化过去
}

func Init() {
	Platform = &Core{}
}
