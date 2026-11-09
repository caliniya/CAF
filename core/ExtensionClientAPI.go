package Core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ClientExtension struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	AdapterID string `json:"adapterId,omitempty"`
}

// ExtensionClient 是应用管理的数据容器，不引用外部工作区。
type ExtensionClient struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Extensions []ClientExtension `json:"extensions"`
}

var extensionClientID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var extensionCommandName = regexp.MustCompile(`^[\p{L}\p{N}_-]{1,32}$`)

func validateExtensionClientID(id string) error {
	if !extensionClientID.MatchString(id) {
		return errors.New("ID 需以字母或数字开头，仅支持字母、数字、下划线和短横线，最长 64 个字符")
	}
	upper := strings.ToUpper(id)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" ||
		(len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9') {
		return errors.New("ID 不能使用系统保留的文件夹名称")
	}
	return nil
}

func (c *Core) extensionClientsDirectory() (string, error) {
	if c == nil || c.Paths == nil || c.FS == nil {
		return "", errors.New("客户端存储尚未初始化")
	}
	directory := filepath.Join(c.Paths.Data, "ext", "clients")
	if err := c.FS.CreateDirectory(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}

func (c *Core) readExtensionClientLocked(id string) (*ExtensionClient, error) {
	if err := validateExtensionClientID(id); err != nil {
		return nil, err
	}
	directory, err := c.extensionClientsDirectory()
	if err != nil {
		return nil, err
	}
	data, err := c.FS.ReadFile(filepath.Join(directory, id, "client.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("客户端不存在")
	}
	if err != nil {
		return nil, err
	}
	var client ExtensionClient
	if err := json.Unmarshal(data, &client); err != nil {
		return nil, fmt.Errorf("解析客户端失败：%w", err)
	}
	if client.ID != id || strings.TrimSpace(client.Name) == "" {
		return nil, errors.New("客户端配置中的 ID 或名称无效")
	}
	if client.Extensions == nil {
		client.Extensions = []ClientExtension{}
	}
	names, adapters := map[string]bool{}, map[string]bool{}
	for _, item := range client.Extensions {
		if item.Type != "qq" || !extensionCommandName.MatchString(item.Name) || item.AdapterID == "" || names[item.Name] || adapters[item.AdapterID] {
			return nil, errors.New("客户端扩展配置包含无效或重复的名称、适配器")
		}
		names[item.Name], adapters[item.AdapterID] = true, true
	}
	return &client, nil
}

func (c *Core) listExtensionClientsLocked() ([]ExtensionClient, error) {
	directory, err := c.extensionClientsDirectory()
	if err != nil {
		return nil, err
	}
	entries, err := c.FS.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	clients := []ExtensionClient{}
	for _, entry := range entries {
		if !entry.IsDir() || validateExtensionClientID(entry.Name()) != nil {
			continue
		}
		if _, err := c.FS.Stat(filepath.Join(directory, entry.Name(), "client.json")); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		client, err := c.readExtensionClientLocked(entry.Name())
		if err != nil {
			return nil, err
		}
		clients = append(clients, *client)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ID < clients[j].ID })
	return clients, nil
}

func (c *Core) saveExtensionClientLocked(client *ExtensionClient) error {
	directory, err := c.extensionClientsDirectory()
	if err != nil {
		return err
	}
	return c.writeJSONLocked(filepath.Join(directory, client.ID, "client.json"), client)
}

func (c *Core) ListExtensionClients() ([]ExtensionClient, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	return c.listExtensionClientsLocked()
}

func (c *Core) GetExtensionClient(id string) (*ExtensionClient, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	return c.readExtensionClientLocked(id)
}

func (c *Core) CreateExtensionClient(name, id string) (*ExtensionClient, error) {
	name, id = strings.TrimSpace(name), strings.TrimSpace(id)
	if name == "" {
		return nil, errors.New("客户端名称不能为空")
	}
	if err := validateExtensionClientID(id); err != nil {
		return nil, err
	}
	c.extMu.Lock()
	defer c.extMu.Unlock()
	directory, err := c.extensionClientsDirectory()
	if err != nil {
		return nil, err
	}
	entries, err := c.FS.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), id) {
			return nil, errors.New("ID 已被使用，请使用其他 ID")
		}
	}
	path := filepath.Join(directory, id)
	if err := c.FS.CreateDirectory(path, 0o700); err != nil {
		return nil, err
	}
	client := &ExtensionClient{ID: id, Name: name, Extensions: []ClientExtension{}}
	if err := c.saveExtensionClientLocked(client); err != nil {
		// 创建失败时只清理本次新建的空目录，不递归删除数据。
		_ = c.FS.DeleteFile(filepath.Join(path, "client.json.tmp"))
		_ = c.FS.DeleteFile(path)
		return nil, fmt.Errorf("创建客户端失败：%w", err)
	}
	return client, nil
}

func (c *Core) RenameExtensionClient(id, name string) (*ExtensionClient, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("客户端名称不能为空")
	}
	c.extMu.Lock()
	defer c.extMu.Unlock()
	client, err := c.readExtensionClientLocked(id)
	if err != nil {
		return nil, err
	}
	client.Name = name
	if err := c.saveExtensionClientLocked(client); err != nil {
		return nil, err
	}
	return client, nil
}

// SetExtensionClientQQ 绑定 QQ 实现。启用时启动监听，停用时关闭连接。
func (c *Core) SetExtensionClientQQ(id, adapterID, name string, enabled bool) (*ExtensionClient, error) {
	name = strings.TrimSpace(name)
	if !extensionCommandName.MatchString(name) {
		return nil, errors.New("扩展名称必填，仅支持文字、数字、下划线和短横线，最长 32 个字符")
	}
	c.extMu.Lock()
	defer c.extMu.Unlock()
	client, err := c.readExtensionClientLocked(id)
	if err != nil {
		return nil, err
	}
	var previous ClientExtension
	position := -1
	for i, item := range client.Extensions {
		if item.Type != "qq" {
			continue
		}
		if item.AdapterID == adapterID {
			previous = item
			position = i
		} else if item.Name == name {
			return nil, errors.New("该客户端已存在同名 QQ 扩展")
		}
	}
	if adapterID == "" {
		return nil, errors.New("请先选择或创建 QQ 适配器")
	}
	wasRunning := false
	if adapterID != "" {
		adapter, err := c.GetQQAdapter(adapterID)
		if err != nil {
			return nil, err
		}
		wasRunning = adapter.State != "stopped"
		clients, err := c.listExtensionClientsLocked()
		if err != nil {
			return nil, err
		}
		for _, other := range clients {
			if other.ID == id {
				continue
			}
			for _, item := range other.Extensions {
				if item.Type == "qq" && item.AdapterID == adapterID {
					return nil, fmt.Errorf("该适配器已绑定客户端「%s」", other.Name)
				}
			}
		}
	}
	if enabled {
		if _, err := c.StartQQAdapter(adapterID); err != nil {
			return nil, err
		}
	}
	item := ClientExtension{Type: "qq", Name: name, AdapterID: adapterID, Enabled: enabled}
	if position < 0 {
		client.Extensions = append(client.Extensions, item)
	} else {
		client.Extensions[position] = item
	}
	if err := c.saveExtensionClientLocked(client); err != nil {
		if enabled && !wasRunning {
			_, _ = c.StopQQAdapter(adapterID)
		}
		return nil, err
	}
	if !enabled && previous.AdapterID != "" {
		if _, err := c.StopQQAdapter(previous.AdapterID); err != nil {
			return client, err
		}
	}
	return client, nil
}

func (c *Core) RemoveExtensionClientQQ(id, adapterID string) (*ExtensionClient, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	client, err := c.readExtensionClientLocked(id)
	if err != nil {
		return nil, err
	}
	items := []ClientExtension{}
	found := false
	for _, item := range client.Extensions {
		if item.Type == "qq" && item.AdapterID == adapterID {
			found = true
			continue
		}
		items = append(items, item)
	}
	if !found {
		return nil, errors.New("QQ 扩展未绑定此客户端")
	}
	client.Extensions = items
	if err := c.saveExtensionClientLocked(client); err != nil {
		return nil, err
	}
	_, _ = c.StopQQAdapter(adapterID)
	return client, nil
}

// 删除客户端记录并关闭扩展，内部数据归档保留，ID 不会被重新分配。
func (c *Core) DeleteExtensionClient(id string) error {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	client, err := c.readExtensionClientLocked(id)
	if err != nil {
		return err
	}
	directory, err := c.extensionClientsDirectory()
	if err != nil {
		return err
	}
	path := filepath.Join(directory, id)
	if err := c.FS.MoveFile(filepath.Join(path, "client.json"), filepath.Join(path, "archived.json")); err != nil {
		return err
	}
	for _, item := range client.Extensions {
		if item.Type == "qq" && item.AdapterID != "" {
			_, _ = c.StopQQAdapter(item.AdapterID)
		}
	}
	return nil
}

// StartExtensionClients 恢复已启用扩展的监听。连接仍由外部 QQ 实现发起。
func (c *Core) StartExtensionClients() {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	clients, err := c.listExtensionClientsLocked()
	if err != nil {
		return
	}
	for _, client := range clients {
		for _, item := range client.Extensions {
			if item.Type != "qq" || !item.Enabled {
				continue
			}
			if _, err := c.StartQQAdapter(item.AdapterID); err != nil {
				c.qqMu.Lock()
				if c.qqClients == nil {
					c.qqClients = map[string]*qqAdapterRuntime{}
				}
				c.qqClients[item.AdapterID] = &qqAdapterRuntime{err: err.Error()}
				c.qqMu.Unlock()
			}
		}
	}
}
