package Backend

import (
	"sync"

	"github.com/caliniya/caf/NetClient/NetAPI"
	"github.com/caliniya/caf/core/credentials"
	platform "github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	DeviceKeys     credentials.DeviceKeys
	OnVaultReady   func()
	vaultMu        sync.Mutex
	vaultReadyOnce sync.Once
	vault          *credentials.Store
	internal       *internalModels // non-nil only for a signed-in internal user
	profileRoot    string          // common account data root, outside the file tools
	Paths          *platform.Paths
	Configs        *Configs
	FS             platform.FilePlatform

	extMu    sync.Mutex
	configMu sync.RWMutex // 序列化配置读写；发布后的默认工具配置不再原地修改。

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
