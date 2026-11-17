package Backend

import (
	"sync"

	"github.com/caliniya/caf/NetClient/NetAPI"
	platform "github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	Paths   *platform.Paths
	Configs *Configs
	FS      platform.FilePlatform

	extMu sync.Mutex

	APIClient *NetAPI.NetAPIClient

	Servers
}

func Init() {
	Platform = &Core{
		APIClient: NetAPI.InitNetAPI(),
		Servers:   Servers{},
	}
	Platform.InitServers()
}

// InitServers 注册内置服务，不启动监听或生成，也不要求存储已经初始化。
// 手动创建 Core 的宿主应在调用服务 API 前执行一次，可安全重复调用。
func (c *Core) InitServers() {
	if c != nil {
		c.Servers.initDefaults(c)
	}
}
