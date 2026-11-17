package Backend

import (
	"context"
	"sync"

	"github.com/caliniya/caf/NetClient/NetAPI"
	platform "github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	Paths   *platform.Paths
	Configs *Configs
	FS      platform.FilePlatform
	QQ      QQServer
	Chat    ChatServer

	extMu sync.Mutex

	APIClient *NetAPI.NetAPIClient

	Servers
}

func Init() {
	Platform = &Core{
		APIClient: NetAPI.InitNetAPI(),
		Servers:   Servers{},
	}
	Platform.Servers[QQServerName] = NewQQServer(Platform)
	Platform.Servers[QQServerName].Init(context.Background())
	Platform.Servers[ChatServerName] = NewChatServer(Platform)
	_ = Platform.Chat.Init(context.Background())
}
