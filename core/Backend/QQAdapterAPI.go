package Backend

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
	"time"

	extension "github.com/caliniya/caf/core/Extension"
	onebot11qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	"github.com/caliniya/caf/core/credentials"
	identity "github.com/caliniya/caf/utils/id"
)

const qqAdapterPath = "/onebot/v11/ws"

// QQAdapter 的列表和编辑响应均不包含令牌。
type QQAdapter struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	SelfID    string         `json:"selfId"`
	QQ        string         `json:"qq"`
	Address   string         `json:"address"`
	State     string         `json:"state"`
	LastError string         `json:"lastError,omitempty"`
	HasToken  bool           `json:"hasToken"`
	Port      int            `json:"port"`
	Options   *QQChatOptions `json:"chatOptions,omitempty"`
}

func (c *Core) loadQQAdaptersLocked() error {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return serviceErr
	}

	if c.Paths == nil || c.FS == nil {
		return errors.New("QQ 适配器存储尚未初始化")
	}
	if qqServer.adapters != nil {
		return nil
	}
	data, err := c.FS.ReadFile(filepath.Join(c.Paths.Data, "qq", "adapters.json"))
	if errors.Is(err, fs.ErrNotExist) {
		qqServer.adapters = []qqAdapterConfig{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 QQ 适配器失败：%w", err)
	}
	var stored struct {
		Version  int `json:"version"`
		Adapters []struct {
			qqAdapterConfig
			LegacyToken string `json:"token"`
		} `json:"adapters"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("解析 QQ 适配器失败：%w", err)
	}
	if stored.Version != 1 && stored.Version != 2 {
		return errors.New("不支持的 QQ 适配器配置版本")
	}
	ids, ports := map[string]bool{}, map[int]bool{}
	configs := make([]qqAdapterConfig, 0, len(stored.Adapters))
	changes := map[string]credentials.Entry{}
	for _, item := range stored.Adapters {
		config := item.qqAdapterConfig
		if stored.Version == 1 && strings.TrimSpace(item.LegacyToken) == "" {
			return errors.New("旧 QQ 配置缺少 Token，已保留原文件")
		}
		if item.LegacyToken != "" {
			changes[qqRef(config.ID)] = credentials.Entry{Value: item.LegacyToken}
		}
		configs = append(configs, config)
		if _, _, err := validateQQAdapter(config.Name, config.SelfID); err != nil {
			return fmt.Errorf("QQ 适配器配置无效：%w", err)
		}
		if config.ID == "" || ids[config.ID] || config.Port < 1 || config.Port > 65535 || ports[config.Port] {
			return errors.New("QQ 适配器配置包含无效或重复的 ID / 端口")
		}
		ids[config.ID], ports[config.Port] = true, true
		if config.Options != nil {
			if _, err := parseQQChatOptions(config.Options.parameters(), nil); err != nil {
				return fmt.Errorf("QQ 聊天选项无效：%w", err)
			}
		}
	}
	if stored.Version == 1 || len(changes) > 0 {
		return c.withCredentials(changes, true, func() error { return c.saveQQAdaptersLocked(configs) })
	}
	qqServer.adapters = configs
	return nil
}

func (c *Core) saveQQAdaptersLocked(configs []qqAdapterConfig) error {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return serviceErr
	}

	directory := filepath.Join(c.Paths.Data, "qq")
	if err := c.FS.CreateDirectory(directory, 0o700); err != nil {
		return fmt.Errorf("创建 QQ 适配器目录失败：%w", err)
	}
	stored := struct {
		Version  int               `json:"version"`
		Adapters []qqAdapterConfig `json:"adapters"`
	}{2, configs}
	if err := c.writeJSONLocked(filepath.Join(directory, "adapters.json"), stored); err != nil {
		return fmt.Errorf("保存 QQ 适配器失败：%w", err)
	}
	qqServer.adapters = configs
	return nil
}

func (c *Core) qqAdapterIndexLocked(id string) (int, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return 0, serviceErr
	}

	if err := c.loadQQAdaptersLocked(); err != nil {
		return -1, err
	}
	for i := range qqServer.adapters {
		if qqServer.adapters[i].ID == id {
			return i, nil
		}
	}
	return -1, errors.New("QQ 适配器不存在")
}

func qqAdapterAddress(port int) string {
	return "ws://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + qqAdapterPath
}

func (q *QQServer) qqAdapterListenAddress(port int) string {
	host := q.ListenHost
	if host == "" {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func (c *Core) nextQQAdapterPortLocked() (int, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return 0, serviceErr
	}

	used := map[int]bool{}
	for _, config := range qqServer.adapters {
		used[config.Port] = true
	}
	for port := 6701; port <= 65535; port++ {
		if !used[port] {
			listener, err := net.Listen("tcp", qqServer.qqAdapterListenAddress(port))
			if err == nil {
				_ = listener.Close()
				return port, nil
			}
		}
	}
	return 0, errors.New("没有可分配的 QQ 适配器端口")
}

func (q *QQServer) qqAdapterViewLocked(config qqAdapterConfig) QQAdapter {
	view := QQAdapter{
		ID: config.ID, Name: config.Name, SelfID: config.SelfID, HasToken: true,
		QQ: config.SelfID, Address: qqAdapterAddress(config.Port), State: "stopped", Port: config.Port,
	}
	if view.QQ == "" {
		view.QQ = config.DetectedQQ
	}
	if running := q.clients[config.ID]; running != nil {
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
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	if err := c.loadQQAdaptersLocked(); err != nil {
		return nil, err
	}
	views := make([]QQAdapter, 0, len(qqServer.adapters))
	for _, config := range qqServer.adapters {
		views = append(views, qqServer.qqAdapterViewLocked(config))
	}
	return views, nil
}

func (c *Core) GetQQAdapter(id string) (*QQAdapter, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	view := qqServer.qqAdapterViewLocked(qqServer.adapters[i])
	options := copyQQChatOptions(qqServer.adapters[i].Options)
	view.Options = &options
	return &view, nil
}

// GetQQAdapterAddress 给创建窗口预览地址；创建时再次分配，避免并发创建冲突。
func (c *Core) GetQQAdapterAddress() (string, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return "", serviceErr
	}

	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	if err := c.loadQQAdaptersLocked(); err != nil {
		return "", err
	}
	port, err := c.nextQQAdapterPortLocked()
	if err != nil {
		return "", err
	}
	return qqAdapterAddress(port), nil
}

func validateQQAdapter(name, selfID string) (string, string, error) {
	name, selfID = strings.TrimSpace(name), strings.TrimSpace(selfID)
	if name == "" {
		return "", "", errors.New("适配器名称不能为空")
	}
	if selfID != "" {
		id, err := strconv.ParseInt(selfID, 10, 64)
		if err != nil || id <= 0 {
			return "", "", errors.New("QQ 号必须是正整数")
		}
		selfID = strconv.FormatInt(id, 10)
	}
	return name, selfID, nil
}

func (c *Core) CreateQQAdapter(name, token, selfID string) (*QQAdapter, error) {
	return c.createQQAdapter(name, token, selfID, 0)
}

func (c *Core) createQQAdapter(name, token, selfID string, port int, chatOptions ...*QQChatOptions) (*QQAdapter, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	token = strings.TrimSpace(token)
	name, selfID, err := validateQQAdapter(name, selfID)
	if err != nil {
		return nil, err
	}
	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
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
	if token == "" {
		return nil, errors.New("Token 不能为空")
	}
	adapterID, err := identity.New(identity.QQAdapter)
	if err != nil {
		return nil, err
	}
	config := qqAdapterConfig{ID: adapterID, Name: name, SelfID: selfID, Port: port}
	if len(chatOptions) > 0 {
		config.Options = chatOptions[0]
	}
	configs := append(append([]qqAdapterConfig{}, qqServer.adapters...), config)
	if err := c.withCredentials(map[string]credentials.Entry{qqRef(adapterID): {Value: token}}, false, func() error { return c.saveQQAdaptersLocked(configs) }); err != nil {
		return nil, err
	}
	view := qqServer.qqAdapterViewLocked(config)
	return &view, nil
}

func (c *Core) UpdateQQAdapter(id, name, token, selfID string) (*QQAdapter, error) {
	return c.updateQQAdapter(id, name, token, selfID, 0)
}

func (c *Core) updateQQAdapter(id, name, token, selfID string, port int, chatOptions ...*QQChatOptions) (*QQAdapter, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	token = strings.TrimSpace(token)
	name, selfID, err := validateQQAdapter(name, selfID)
	if err != nil {
		return nil, err
	}
	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	previous := qqServer.adapters[i]
	if port == 0 {
		port = previous.Port
	}
	if err := c.validateQQPortLocked(port, id); err != nil {
		return nil, err
	}
	changes := map[string]credentials.Entry{}
	if token != "" {
		old, err := c.credential(qqRef(id))
		if err != nil && !errors.Is(err, credentials.ErrNoEntry) {
			return nil, err
		}
		if old.Value != token {
			changes[qqRef(id)] = credentials.Entry{Value: token}
		}
	}
	if running := qqServer.clients[id]; running != nil && running.client != nil &&
		(len(changes) > 0 || previous.SelfID != selfID || previous.Port != port) {
		return nil, errors.New("请先停用扩展，再修改 Token、QQ 号或端口")
	}
	configs := append([]qqAdapterConfig{}, qqServer.adapters...)
	configs[i].Name, configs[i].SelfID = name, selfID
	configs[i].Port = port
	if len(chatOptions) > 0 {
		configs[i].Options = chatOptions[0]
	}
	if previous.SelfID != selfID {
		configs[i].DetectedQQ = ""
	}
	if err := c.withCredentials(changes, false, func() error { return c.saveQQAdaptersLocked(configs) }); err != nil {
		return nil, err
	}
	view := qqServer.qqAdapterViewLocked(configs[i])
	return &view, nil
}

func (c *Core) validateQQPortLocked(port int, exceptID string) error {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return serviceErr
	}

	if port < 1 || port > 65535 {
		return errors.New("监听端口必须在 1 到 65535 之间")
	}
	for _, config := range qqServer.adapters {
		if config.ID != exceptID && config.Port == port {
			return errors.New("该端口已被其他扩展使用")
		}
	}
	return nil
}

func (c *Core) StartQQAdapter(id string) (*QQAdapter, error) {
	return c.startQQAdapter(context.Background(), id)
}

func (c *Core) startQQAdapter(parent context.Context, id string) (*QQAdapter, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	if err := parent.Err(); err != nil {
		return nil, err
	}
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	config := qqServer.adapters[i]
	if running := qqServer.clients[id]; running != nil && running.client != nil {
		view := qqServer.qqAdapterViewLocked(config)
		return &view, nil
	}
	secret, err := c.credential(qqRef(id))
	if err != nil {
		return nil, err
	}
	client, err := onebot11qq.NewReverseWSExtension(onebot11qq.ReverseWSConfig{
		Address: qqServer.qqAdapterListenAddress(config.Port),
		Path:    qqAdapterPath, AccessToken: secret.Value, SelfID: config.SelfID,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	if err := client.Start(ctx); err != nil {
		cancel()
		_ = client.Close()
		return nil, err
	}
	if qqServer.clients == nil {
		qqServer.clients = map[string]*qqAdapterRuntime{}
	}
	running := &qqAdapterRuntime{client: client, cancel: cancel, done: make(chan struct{})}
	qqServer.clients[id] = running
	go func() { defer close(running.done); c.consumeQQAdapterEvents(ctx, id, client) }()
	view := qqServer.qqAdapterViewLocked(config)
	return &view, nil
}

// 持续消费事件，避免消息堆积阻塞连接；上层可订阅 qq:event 继续分发。
func (c *Core) consumeQQAdapterEvents(ctx context.Context, id string, client onebot11qq.Extension) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return
	}

	commands := make(chan qqCommandJob, 256)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); c.runQQCommands(ctx, id, client, commands) }()
	defer func() {
		qqServer.mu.Lock()
		if running := qqServer.clients[id]; running != nil && running.client == client {
			running.cancel()
		}
		qqServer.mu.Unlock()
		close(commands)
		<-workerDone
	}()
	for envelope := range client.Events() {
		var event onebot11qq.Event
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			continue
		}
		qqServer.mu.Lock()
		running := qqServer.clients[id]
		if running == nil || running.client != client {
			qqServer.mu.Unlock()
			continue
		}
		if event.Info != nil && event.Info.SelfID != "" {
			for i, config := range qqServer.adapters {
				if config.ID == id && config.DetectedQQ != event.Info.SelfID {
					configs := append([]qqAdapterConfig{}, qqServer.adapters...)
					configs[i].DetectedQQ = event.Info.SelfID
					if err := c.saveQQAdaptersLocked(configs); err != nil {
						running.err = err.Error()
					}
					break
				}
			}
		}
		qqServer.mu.Unlock()
		if event.Message != nil {
			select {
			case commands <- qqCommandJob{Message: *event.Message, ReceivedAt: time.Now()}:
			default:
				qqServer.mu.Lock()
				if running := qqServer.clients[id]; running != nil && running.client == client {
					running.err = "QQ 消息队列已满，当前消息未提交对话处理"
				}
				qqServer.mu.Unlock()
			}
		}
		if Emit != nil {
			Emit("extension:event", struct {
				ExtensionID string          `json:"extensionId"`
				Event       extension.Event `json:"event"`
			}{id, envelope})
			Emit("qq:event", struct {
				AdapterID string           `json:"adapterId"`
				Event     onebot11qq.Event `json:"event"`
			}{id, event})
		}
	}
	qqServer.mu.Lock()
	if running := qqServer.clients[id]; running != nil && running.client == client {
		running.cancel()
		running.err = client.Info().LastError
		running.client = nil
	}
	qqServer.mu.Unlock()
}

func (c *Core) StopQQAdapter(id string) (*QQAdapter, error) {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return nil, err
	}
	if running := qqServer.clients[id]; running != nil && running.client != nil {
		running.cancel()
		if err := running.client.Close(); err != nil {
			return nil, err
		}
	}
	delete(qqServer.clients, id)
	view := qqServer.qqAdapterViewLocked(qqServer.adapters[i])
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
	return c.deleteQQAdapterLocked(id)
}

// 调用方持有 extMu，确保删除期间客户端归属不会发生变化。
func (c *Core) deleteQQAdapterLocked(id string) error {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return serviceErr
	}

	qqServer.mu.Lock()
	defer qqServer.mu.Unlock()
	i, err := c.qqAdapterIndexLocked(id)
	if err != nil {
		return err
	}
	configs := append([]qqAdapterConfig{}, qqServer.adapters[:i]...)
	configs = append(configs, qqServer.adapters[i+1:]...)
	if err := c.withCredentials(map[string]credentials.Entry{qqRef(id): {}}, false, func() error { return c.saveQQAdaptersLocked(configs) }); err != nil {
		return err
	}
	c.closeQQChatStorageLocked(id)
	if running := qqServer.clients[id]; running != nil && running.client != nil {
		running.cancel()
		_ = running.client.Close()
	}
	delete(qqServer.clients, id)
	return nil
}

func (c *Core) CloseQQAdapters() {
	qqServer, serviceErr := c.Servers.Get[*QQServer](QQServerName)
	if serviceErr != nil {
		return
	}

	_ = qqServer.Close(context.Background())
}
