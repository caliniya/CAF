package Core

import (
	"sync"

	chat "github.com/caliniya/caf/core/Chat"
	platform "github.com/caliniya/caf/platform"
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

	ChatList map[string]chat.ChatTask

	qqTestLifecycle sync.Mutex
	qqTestMu        sync.Mutex
	qqTest          *qqTestSession
	qqTestState     QQTestState
	qqTestLog       []QQTestLogEntry
	qqTestLogID     uint64
}

func (c *Core) GetPaths() *platform.Paths {
	return c.Paths // Paths 有 json tag,可以直接序列化过去
}

func Init() {
	Platform = &Core{}
}
