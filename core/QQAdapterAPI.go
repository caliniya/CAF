package Core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	extension "github.com/caliniya/caf/core/Extension"
	qq "github.com/caliniya/caf/core/Extension/QQ"
)

const qqAdapterPath = "/onebot/v11/ws"

type qqAdapterConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Token      string `json:"token"`
	SelfID     string `json:"selfId,omitempty"`
	DetectedQQ string `json:"detectedQQ,omitempty"`
	Port       int    `json:"port"`
}

type qqAdapterRuntime struct {
	client qq.Extension
	err    string
	cancel context.CancelFunc
}

// QQAdapter 的列表响应不包含令牌；编辑时通过 GetQQAdapter 单独读取。
type QQAdapter struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SelfID    string `json:"selfId"`
	QQ        string `json:"qq"`
	Address   string `json:"address"`
	State     string `json:"state"`
	LastError string `json:"lastError,omitempty"`
	Token     string `json:"token,omitempty"`
	Port      int    `json:"port"`
}

func (c *Core) loadQQAdaptersLocked() error {
	if c.Paths == nil || c.FS == nil {
		return errors.New("QQ 适配器存储尚未初始化")
	}
	if c.qqAdapters != nil {
		return nil
	}
	data, err := c.FS.ReadFile(filepath.Join(c.Paths.Data, "qq", "adapters.json"))
	if errors.Is(err, fs.ErrNotExist) {
		c.qqAdapters = []qqAdapterConfig{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 QQ 适配器失败：%w", err)
	}
	var stored struct {
		Version  int               `json:"version"`
		Adapters []qqAdapterConfig `json:"adapters"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("解析 QQ 适配器失败：%w", err)
	}
	if stored.Version != 1 {
		return errors.New("不支持的 QQ 适配器配置版本")
	}
	ids, ports := map[string]bool{}, map[int]bool{}
	for _, config := range stored.Adapters {
		if _, _, _, err := validateQQAdapter(config.Name, config.Token, config.SelfID); err != nil {
			return fmt.Errorf("QQ 适配器配置无效：%w", err)
		}
		if config.ID == "" || ids[config.ID] || config.Port < 1 || config.Port > 65535 || ports[config.Port] {
			return errors.New("QQ 适配器配置包含无效或重复的 ID / 端口")
		}
		ids[config.ID], ports[config.Port] = true, true
	}
	c.qqAdapters = stored.Adapters
	if c.qqAdapters == nil {
		c.qqAdapters = []qqAdapterConfig{}
	}
	return nil
}

func (c *Core) saveQQAdaptersLocked(configs []qqAdapterConfig) error {
	directory := filepath.Join(c.Paths.Data, "qq")
	if err := c.FS.CreateDirectory(directory, 0o700); err != nil {
		return fmt.Errorf("创建 QQ 适配器目录失败：%w", err)
	}
	stored := struct {
		Version  int               `json:"version"`
		Adapters []qqAdapterConfig `json:"adapters"`
	}{1, configs}
	if err := c.writeJSONLocked(filepath.Join(directory, "adapters.json"), stored); err != nil {
		return fmt.Errorf("保存 QQ 适配器失败：%w", err)
	}
	c.qqAdapters = configs
	return nil
}

func (c *Core) qqAdapterIndexLocked(id string) (int, error) {
	if err := c.loadQQAdaptersLocked(); err != nil {
		return -1, err
	}
	for i := range c.qqAdapters {
		if c.qqAdapters[i].ID == id {
			return i, nil
		}
	}
	return -1, errors.New("QQ 适配器不存在")
}

func qqAdapterAddress(port int) string {
	return "ws://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + qqAdapterPath
}

func (c *Core) nextQQAdapterPortLocked() (int, error) {
	used := map[int]bool{}
	for _, config := range c.qqAdapters {
		used[config.Port] = true
	}
	for port := 6701; port <= 65535; port++ {
		if !used[port] {
			listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
			if err == nil {
				_ = listener.Close()
				return port, nil
			}
		}
	}
	return 0, errors.New("没有可分配的 QQ 适配器端口")
}

func (c *Core) qqAdapterViewLocked(config qqAdapterConfig) QQAdapter {
	view := QQAdapter{
		ID: config.ID, Name: config.Name, SelfID: config.SelfID,
		QQ: config.SelfID, Address: qqAdapterAddress(config.Port), State: "stopped", Port: config.Port,
	}
	if view.QQ == "" {
		view.QQ = config.DetectedQQ
	}
	if running := c.qqClients[config.ID]; running != nil {
		view.LastError = running.err
		if running.client != nil {
			info := running.client.Info()
			view.State = "disconnected"
			if info.Connected {
				view.State = "connected"
			}
			if info.SelfID != "" {
				view.QQ = info.SelfID
			}
			if view.LastError == "" && info.LastError != "" {
				view.LastError = info.LastError
			}
		}
	}
	return view
}

func (c *Core) ListQQAdapters() ([]QQAdapter, error) {
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	if err := c.loadQQAdaptersLocked(); err != nil {
		return nil, err
	}
	views := make([]QQAdapter, 0, len(c.qqAdapters))
	for _, config := range c.qqAdapters {
		views = append(views, c.qqAdapterViewLocked(config))
	}
	return views, nil
}

func (c *Core) GetQQAdapter(id string) (*QQAdapter, error) {
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	view := c.qqAdapterViewLocked(c.qqAdapters[i])
	view.Token = c.qqAdapters[i].Token
	return &view, nil
}

// GetQQAdapterAddress 给创建窗口预览地址；创建时再次分配，避免并发创建冲突。
func (c *Core) GetQQAdapterAddress() (string, error) {
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	if err := c.loadQQAdaptersLocked(); err != nil {
		return "", err
	}
	port, err := c.nextQQAdapterPortLocked()
	if err != nil {
		return "", err
	}
	return qqAdapterAddress(port), nil
}

func validateQQAdapter(name, token, selfID string) (string, string, string, error) {
	name, token, selfID = strings.TrimSpace(name), strings.TrimSpace(token), strings.TrimSpace(selfID)
	if name == "" {
		return "", "", "", errors.New("适配器名称不能为空")
	}
	if token == "" {
		return "", "", "", errors.New("Token 不能为空")
	}
	if selfID != "" {
		id, err := strconv.ParseInt(selfID, 10, 64)
		if err != nil || id <= 0 {
			return "", "", "", errors.New("QQ 号必须是正整数")
		}
		selfID = strconv.FormatInt(id, 10)
	}
	return name, token, selfID, nil
}

func (c *Core) CreateQQAdapter(name, token, selfID string) (*QQAdapter, error) {
	return c.createQQAdapter(name, token, selfID, 0)
}

func (c *Core) createQQAdapter(name, token, selfID string, port int) (*QQAdapter, error) {
	name, token, selfID, err := validateQQAdapter(name, token, selfID)
	if err != nil {
		return nil, err
	}
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	if err := c.loadQQAdaptersLocked(); err != nil {
		return nil, err
	}
	if port == 0 {
		port, err = c.nextQQAdapterPortLocked()
		if err != nil {
			return nil, err
		}
	} else if err := c.validateQQPortLocked(port, ""); err != nil {
		return nil, err
	}
	config := qqAdapterConfig{ID: newID(), Name: name, Token: token, SelfID: selfID, Port: port}
	configs := append(append([]qqAdapterConfig{}, c.qqAdapters...), config)
	if err := c.saveQQAdaptersLocked(configs); err != nil {
		return nil, err
	}
	view := c.qqAdapterViewLocked(config)
	return &view, nil
}

func (c *Core) UpdateQQAdapter(id, name, token, selfID string) (*QQAdapter, error) {
	return c.updateQQAdapter(id, name, token, selfID, 0)
}

func (c *Core) updateQQAdapter(id, name, token, selfID string, port int) (*QQAdapter, error) {
	name, token, selfID, err := validateQQAdapter(name, token, selfID)
	if err != nil {
		return nil, err
	}
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	previous := c.qqAdapters[i]
	if port == 0 {
		port = previous.Port
	}
	if err := c.validateQQPortLocked(port, id); err != nil {
		return nil, err
	}
	if running := c.qqClients[id]; running != nil && running.client != nil &&
		(previous.Token != token || previous.SelfID != selfID || previous.Port != port) {
		return nil, errors.New("请先停用扩展，再修改 Token、QQ 号或端口")
	}
	configs := append([]qqAdapterConfig{}, c.qqAdapters...)
	configs[i].Name, configs[i].Token, configs[i].SelfID = name, token, selfID
	configs[i].Port = port
	if previous.SelfID != selfID {
		configs[i].DetectedQQ = ""
	}
	if err := c.saveQQAdaptersLocked(configs); err != nil {
		return nil, err
	}
	view := c.qqAdapterViewLocked(configs[i])
	return &view, nil
}

func (c *Core) validateQQPortLocked(port int, exceptID string) error {
	if port < 1 || port > 65535 {
		return errors.New("监听端口必须在 1 到 65535 之间")
	}
	for _, config := range c.qqAdapters {
		if config.ID != exceptID && config.Port == port {
			return errors.New("该端口已被其他扩展使用")
		}
	}
	return nil
}

func (c *Core) StartQQAdapter(id string) (*QQAdapter, error) {
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	config := c.qqAdapters[i]
	if running := c.qqClients[id]; running != nil && running.client != nil {
		view := c.qqAdapterViewLocked(config)
		return &view, nil
	}
	client, err := qq.NewReverseWSExtension(qq.ReverseWSConfig{
		Address: net.JoinHostPort("0.0.0.0", strconv.Itoa(config.Port)),
		Path:    qqAdapterPath, AccessToken: config.Token, SelfID: config.SelfID,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := client.Start(ctx); err != nil {
		cancel()
		_ = client.Close()
		return nil, err
	}
	if c.qqClients == nil {
		c.qqClients = map[string]*qqAdapterRuntime{}
	}
	c.qqClients[id] = &qqAdapterRuntime{client: client, cancel: cancel}
	go c.consumeQQAdapterEvents(ctx, id, client)
	view := c.qqAdapterViewLocked(config)
	return &view, nil
}

// 持续消费事件，避免消息堆积阻塞连接；上层可订阅 qq:event 继续分发。
func (c *Core) consumeQQAdapterEvents(ctx context.Context, id string, client qq.Extension) {
	commands := make(chan qqCommandJob, 32)
	go c.runQQCommands(ctx, id, client, commands)
	defer func() {
		c.qqMu.Lock()
		defer c.qqMu.Unlock()
		if running := c.qqClients[id]; running != nil && running.client == client {
			running.cancel()
		}
	}()
	for envelope := range client.Events() {
		var event qq.Event
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			continue
		}
		c.qqMu.Lock()
		running := c.qqClients[id]
		if running == nil || running.client != client {
			c.qqMu.Unlock()
			continue
		}
		if event.Info != nil && event.Info.SelfID != "" {
			for i, config := range c.qqAdapters {
				if config.ID == id && config.DetectedQQ != event.Info.SelfID {
					configs := append([]qqAdapterConfig{}, c.qqAdapters...)
					configs[i].DetectedQQ = event.Info.SelfID
					if err := c.saveQQAdaptersLocked(configs); err != nil {
						running.err = err.Error()
					}
					break
				}
			}
		}
		c.qqMu.Unlock()
		if event.Message != nil {
			select {
			case commands <- qqCommandJob{Message: *event.Message}:
			default:
			}
		}
		if Emit != nil {
			Emit("extension:event", struct {
				ExtensionID string          `json:"extensionId"`
				Event       extension.Event `json:"event"`
			}{id, envelope})
			Emit("qq:event", struct {
				AdapterID string   `json:"adapterId"`
				Event     qq.Event `json:"event"`
			}{id, event})
		}
	}
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	if running := c.qqClients[id]; running != nil && running.client == client {
		running.cancel()
		running.err = client.Info().LastError
		running.client = nil
	}
}

func (c *Core) StopQQAdapter(id string) (*QQAdapter, error) {
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	if running := c.qqClients[id]; running != nil && running.client != nil {
		running.cancel()
		if err := running.client.Close(); err != nil {
			return nil, err
		}
	}
	delete(c.qqClients, id)
	view := c.qqAdapterViewLocked(c.qqAdapters[i])
	return &view, nil
}

func (c *Core) DeleteQQAdapter(id string) error {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	clients, err := c.listExtensionClientsLocked()
	if err != nil {
		return err
	}
	for _, owner := range clients {
		for _, item := range owner.Extensions {
			if item.AdapterID == id {
				return errors.New("请先在客户端中解除 QQ 适配器绑定，再删除适配器")
			}
		}
	}
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return err
	}
	configs := append([]qqAdapterConfig{}, c.qqAdapters[:i]...)
	configs = append(configs, c.qqAdapters[i+1:]...)
	if err := c.saveQQAdaptersLocked(configs); err != nil {
		return err
	}
	if running := c.qqClients[id]; running != nil && running.client != nil {
		running.cancel()
		_ = running.client.Close()
	}
	delete(c.qqClients, id)
	return nil
}

func (c *Core) CloseQQAdapters() {
	c.qqMu.Lock()
	defer c.qqMu.Unlock()
	for id, running := range c.qqClients {
		if running.cancel != nil {
			running.cancel()
		}
		if running.client != nil {
			_ = running.client.Close()
		}
		delete(c.qqClients, id)
	}
}
