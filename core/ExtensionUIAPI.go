package Core

import (
	"errors"
	"strconv"
	"strings"

	extension "github.com/caliniya/caf/core/Extension"
	qq "github.com/caliniya/caf/core/Extension/QQ"
)

// ExtensionOption 是一个已创建实例，绑定与运行状态分别表示。
type ExtensionOption struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	ClientID   string            `json:"clientId,omitempty"`
	ClientName string            `json:"clientName,omitempty"`
	Enabled    bool              `json:"enabled"`
	Running    bool              `json:"running"`
	Values     map[string]string `json:"values"`
	LastError  string            `json:"lastError,omitempty"`
}

func (c *Core) ListExtensionTypes() []extension.Descriptor {
	return []extension.Descriptor{qq.OneBot11Descriptor()}
}

func (c *Core) ListExtensions() ([]ExtensionOption, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	clients, err := c.listExtensionClientsLocked()
	if err != nil {
		return nil, err
	}
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	if err := c.loadQQAdaptersLocked(); err != nil {
		return nil, err
	}
	options := make([]ExtensionOption, 0, len(c.qqAdapters))
	for _, config := range c.qqAdapters {
		view := c.qqAdapterViewLocked(config)
		label := "未启动"
		if view.State == "disconnected" {
			label = "未连接"
		} else if view.State == "connected" {
			label = "已连接"
		}
		values := map[string]string{"status": label, "address": view.Address}
		if running := c.qqClients[config.ID]; running != nil && running.client != nil {
			values = running.client.UIValues()
		}
		option := ExtensionOption{ID: config.ID, Type: qq.ExtensionType, Name: config.Name, Running: view.State != "stopped", Values: values, LastError: view.LastError}
		for _, client := range clients {
			for _, binding := range client.Extensions {
				if binding.AdapterID == config.ID {
					option.ClientID, option.ClientName, option.Name, option.Enabled = client.ID, client.Name, binding.Name, binding.Enabled
				}
			}
		}
		options = append(options, option)
	}
	return options, nil
}

func (c *Core) GetExtensionParameters(id string) (map[string]string, error) {
	adapter, err := c.GetQQAdapter(id)
	if err != nil {
		return nil, err
	}
	// 已绑定实例使用其命令名称作为表单名称。
	options, err := c.ListExtensions()
	if err != nil {
		return nil, err
	}
	for _, option := range options {
		if option.ID == id {
			adapter.Name = option.Name
		}
	}
	return map[string]string{"name": adapter.Name, "token": adapter.Token, "selfId": adapter.SelfID, "port": strconv.Itoa(adapter.Port)}, nil
}

func parseOneBot11Parameters(parameters map[string]string) (string, string, string, int, error) {
	name := strings.TrimSpace(parameters["name"])
	if !extensionCommandName.MatchString(name) {
		return "", "", "", 0, errors.New("名称必填，仅支持文字、数字、下划线和短横线，最长 32 个字符")
	}
	port, err := strconv.Atoi(parameters["port"])
	if err != nil || port < 1 || port > 65535 {
		return "", "", "", 0, errors.New("监听端口必须在 1 到 65535 之间")
	}
	return name, parameters["token"], parameters["selfId"], port, nil
}

func (c *Core) CreateExtension(kind string, parameters map[string]string) (*ExtensionOption, error) {
	if kind != qq.ExtensionType {
		return nil, errors.New("不支持的扩展类型")
	}
	name, token, selfID, port, err := parseOneBot11Parameters(parameters)
	if err != nil {
		return nil, err
	}
	adapter, err := c.createQQAdapter(name, token, selfID, port)
	if err != nil {
		return nil, err
	}
	return &ExtensionOption{ID: adapter.ID, Type: kind, Name: adapter.Name, Values: map[string]string{"status": "未启动", "address": adapter.Address}}, nil
}

// UpdateExtensionParameters 同步实例参数及绑定名称，失败时恢复已写入的适配器配置。
func (c *Core) UpdateExtensionParameters(id string, parameters map[string]string) error {
	name, token, selfID, port, err := parseOneBot11Parameters(parameters)
	if err != nil {
		return err
	}
	c.extMu.Lock()
	defer c.extMu.Unlock()
	clients, err := c.listExtensionClientsLocked()
	if err != nil {
		return err
	}
	var owner *ExtensionClient
	for i := range clients {
		for j := range clients[i].Extensions {
			binding := &clients[i].Extensions[j]
			if binding.AdapterID == id {
				owner = &clients[i]
				binding.Name = name
			}
		}
	}
	if owner != nil {
		for _, binding := range owner.Extensions {
			if binding.AdapterID != id && binding.Name == name {
				return errors.New("该客户端已存在同名扩展")
			}
		}
	}
	previous, err := c.GetQQAdapter(id)
	if err != nil {
		return err
	}
	if _, err := c.updateQQAdapter(id, name, token, selfID, port); err != nil {
		return err
	}
	if owner != nil {
		if err := c.saveExtensionClientLocked(owner); err != nil {
			_, _ = c.updateQQAdapter(id, previous.Name, previous.Token, previous.SelfID, previous.Port)
			return err
		}
	}
	return nil
}

func (c *Core) SetClientExtensionEnabled(clientID, extensionID string, enabled bool) (*ExtensionClient, error) {
	parameters, err := c.GetExtensionParameters(extensionID)
	if err != nil {
		return nil, err
	}
	return c.SetExtensionClientQQ(clientID, extensionID, parameters["name"], enabled)
}

func (c *Core) DeleteExtension(id string) error { return c.DeleteQQAdapter(id) }
