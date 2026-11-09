package Core

import (
	"context"
	"fmt"
	"time"

	qq "github.com/caliniya/caf/core/Extension/QQ"
)

type qqCommandJob struct {
	Message qq.QQMessage
}

func (c *Core) qqCommandOwner(adapterID string) (*ExtensionClient, ClientExtension) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	clients, err := c.listExtensionClientsLocked()
	if err != nil {
		return nil, ClientExtension{}
	}
	for _, client := range clients {
		for _, item := range client.Extensions {
			if item.Type == qq.ExtensionType && item.Enabled && item.AdapterID == adapterID {
				return &client, item
			}
		}
	}
	return nil, ClientExtension{}
}

// 明确指令使用固定逻辑回复；不调用模型，也不发起主动对话。
func (c *Core) runQQCommands(ctx context.Context, adapterID string, client qq.Extension, jobs <-chan qqCommandJob) {
	seen := map[string]bool{}
	order := []string{}
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			if ctx.Err() != nil {
				return
			}
			owner, binding := c.qqCommandOwner(adapterID)
			if owner == nil {
				continue
			}
			command, ok := qq.ParseCommand(job.Message, binding.Name)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%s:%s:%s:%s", job.Message.Type, job.Message.GroupID, job.Message.UserID, job.Message.ID)
			if job.Message.ID != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
				order = append(order, key)
				if len(order) > 256 {
					delete(seen, order[0])
					order = order[1:]
				}
			}
			text := fmt.Sprintf("可用指令：\n/%s help — 查看帮助\n/%s status — 查看客户端状态", binding.Name, binding.Name)
			if command.Name == "status" && len(command.Args) == 0 {
				text = fmt.Sprintf("客户端：%s\nID：%s\n扩展：%s\nQQ：%s", owner.Name, owner.ID, binding.Name, client.Info().SelfID)
			} else if command.Name != "help" || len(command.Args) > 0 {
				text = "无法识别该指令。\n" + text
			}
			reply := qq.QQMessage{Type: job.Message.Type, UserID: job.Message.UserID, GroupID: job.Message.GroupID, Segments: []qq.QQMessageSegment{qq.QQText(text)}}
			callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := client.SendMessage(callCtx, reply)
			cancel()
			if err != nil && ctx.Err() == nil {
				c.qqMu.Lock()
				if running := c.qqClients[adapterID]; running != nil && running.client == client {
					running.err = err.Error()
				}
				c.qqMu.Unlock()
			}
		}
	}
}
