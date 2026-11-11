package Backend

import (
	"sync"

	//"github.com/caliniya/caf/NetClient"
	"github.com/caliniya/caf/NetClient/NetAPI"
	chat "github.com/caliniya/caf/core/Chat"
	platform "github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	Paths   *platform.Paths
	Configs *Configs
	FS      platform.FilePlatform
	QQ      QQState

	chatMu              sync.Mutex
	chatSending         map[string]bool
	interactionMu       sync.Mutex
	interactionRequests map[string]*pendingQuestion

	extMu sync.Mutex

	ChatList map[string]chat.ChatTask

	APIClient *NetAPI.NetAPIClient
}

func Init() {
	Platform = &Core{
		APIClient: NetAPI.InitNetAPI(),
	}
}
