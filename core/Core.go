package core

import (
	//netclient "github.com/caliniya/caf/NetClient"
	"sync"

	"github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	QQPlatform platform.QQPlatform
	Paths      *platform.Paths
	Configs    *Configs
	//APIClients *[]netclient.APIClient
	FS platform.FilePlatform

	chatMu      sync.Mutex
	chatSending map[string]bool
}

func (c *Core) GetPaths() *platform.Paths {
	return c.Paths // Paths 有 json tag,可以直接序列化过去
}

func Init() {
	Platform = &Core{}
}
