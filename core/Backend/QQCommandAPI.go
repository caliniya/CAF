package Backend

import (
	"context"
	"time"

	onebot11qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
)

type qqCommandJob struct {
	Message    onebot11qq.QQMessage
	ReceivedAt time.Time
}

func (c *Core) qqCommandOwner(adapterID string) (*ExtensionClient, ClientExtension) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	return c.qqCommandOwnerLocked(adapterID)
}

func (c *Core) qqCommandOwnerLocked(adapterID string) (*ExtensionClient, ClientExtension) {
	clients, err := c.listExtensionClientsLocked()
	if err != nil {
		return nil, ClientExtension{}
	}
	for _, client := range clients {
		for _, item := range client.Extensions {
			if item.Type == onebot11qq.ExtensionType && item.Enabled && item.AdapterID == adapterID {
				return &client, item
			}
		}
	}
	return nil, ClientExtension{}
}

// 连接事件消费者只负责入队；指令路由和每个上下文的生成独立执行。
func (c *Core) runQQCommands(ctx context.Context, adapterID string, client onebot11qq.Extension, jobs <-chan qqCommandJob) {
	var runtime *qqChatRuntime
	defer func() {
		if runtime != nil {
			runtime.shutdown()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case job, open := <-jobs:
			if !open {
				return
			}
			if ctx.Err() != nil {
				return
			}
			if runtime == nil {
				var err error
				runtime, err = c.newQQChatRuntime(ctx, adapterID, client)
				if err != nil || runtime == nil {
					if err != nil {
						if server, serviceErr := c.Servers.Get[*QQServer](QQServerName); serviceErr == nil {
							server.mu.Lock()
							if running := server.clients[adapterID]; running != nil && running.client == client {
								running.err = err.Error()
							}
							server.mu.Unlock()
						}
					}
					continue
				}
			}
			runtime.handle(job)
		}
	}
}
