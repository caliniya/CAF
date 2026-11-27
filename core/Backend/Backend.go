package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	netclient "github.com/caliniya/caf/NetClient"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	persona "github.com/caliniya/caf/core/Persona"
	"github.com/caliniya/caf/core/credentials"
	identity "github.com/caliniya/caf/utils/id"
)

// SelectDirectory 供前端调用平台文件夹选择器；取消时返回空字符串。
func (c *Core) SelectDirectory(defaultDirectory string) (string, error) {
	if c == nil || c.FS == nil {
		return "", errors.New("文件系统尚未初始化")
	}
	return c.FS.SelectDirectory(defaultDirectory)
}

// LoadConfigs 读取配置；首次启动时创建空配置和所需目录。
func (c *Core) LoadConfigs() (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	if c.Paths == nil || c.Paths.ConfigFile == "" {
		return nil, errors.New("配置路径尚未初始化")
	}
	if c.FS == nil {
		return nil, errors.New("文件系统尚未初始化")
	}
	if err := c.loadConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs.snapshot(), nil
}

// configAt 校验前置状态并返回第 index 个配置的指针。
func (c *Core) configAt(index int) (*Config, error) {
	if c == nil || c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("配置尚未加载")
	}

	all := *c.Configs.ALLConfig
	if index < 0 || index >= len(all) {
		return nil, fmt.Errorf("配置下标越界：%d（共 %d 项）", index, len(all))
	}
	return &all[index], nil
}

// SelectConfig 供前端调用：启用第 index 个配置（提供商，从 0 开始）并写回配置文件。
// 启用模型随配置走：切换提供商后沿用该配置自己记住的 EnabledModel。
func (c *Core) SelectConfig(index int) (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	selected, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousConfig := c.Configs.EnablesConfig
	c.Configs.EnablesConfig = selected

	if err := c.saveConfig(); err != nil {
		c.Configs.EnablesConfig = previousConfig
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs.snapshot(), nil
}

// SelectModel 供前端调用：在当前启用的配置里选定要使用的模型，
// 记录到该配置的 EnabledModel 字段并写回配置文件。
func (c *Core) SelectModel(model string) (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	if c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("配置尚未加载")
	}
	if c.Configs.EnablesConfig == nil {
		return nil, errors.New("尚未选择配置（提供商），请先选择配置")
	}
	if !slices.ContainsFunc(c.Configs.EnablesConfig.Model, func(item apiImpl.Model) bool {
		return item.ID == model
	}) {
		return nil, fmt.Errorf("模型 %q 不在当前配置的模型列表中", model)
	}

	previousModel := c.Configs.EnablesConfig.EnabledModel
	c.Configs.EnablesConfig.EnabledModel = model

	if err := c.saveConfig(); err != nil {
		c.Configs.EnablesConfig.EnabledModel = previousModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs.snapshot(), nil
}

type ModelProbeResult struct {
	Models        []apiImpl.Model `json:"models"`
	ServerAddress string          `json:"serverAddress"`
}

// ProbeModels 查询模型并探测 Chat Completions 请求地址。
func (c *Core) ProbeModels(serverAddress, apiKey string) (*ModelProbeResult, error) {
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return nil, errors.New("API 地址不能为空")
	}
	client := netclient.NewAPIClient(netclient.Options{
		ServerAddress: serverAddress,
		APIKey:        apiKey,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	endpoint, err := client.ProbeChatEndpoint(ctx)
	if err != nil {
		return nil, err
	}
	models, _ := client.FetchModels(ctx)
	if models == nil {
		models = []apiImpl.Model{}
	}
	return &ModelProbeResult{Models: models, ServerAddress: endpoint}, nil
}

// AddModelConfig 保存新的 API 配置和模型列表，并将其设为当前启用配置。
func (c *Core) AddModelConfig(name, serverAddress, protocol, apiKey string, models []apiImpl.Model) (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	if c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil {
		return nil, errors.New("配置尚未加载")
	}
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return nil, errors.New("API 地址不能为空")
	}
	if protocol == "" {
		protocol = "chat-completions"
	}
	if protocol != "chat-completions" {
		return nil, fmt.Errorf("不支持的协议格式：%s", protocol)
	}
	cleanModels := make([]apiImpl.Model, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		if model.Name == "" {
			model.Name = model.ID
		}
		cleanModels = append(cleanModels, model)
	}
	if len(cleanModels) == 0 {
		return nil, errors.New("至少需要一个模型")
	}

	configID, err := identity.New(identity.Config)
	if err != nil {
		return nil, err
	}
	entry := Config{
		ID:            configID,
		Name:          strings.TrimSpace(name),
		ServerAddress: serverAddress,
		Protocol:      protocol,
		HasAPIKey:     apiKey != "",
		Model:         cleanModels,
		EnabledModel:  cleanModels[0].ID,
	}
	if c.Configs.ALLConfig == nil {
		all := []Config{}
		c.Configs.ALLConfig = &all
	}
	changes := map[string]credentials.Entry{}
	if apiKey != "" {
		v, err := modelSecret(serverAddress, apiKey)
		if err != nil {
			return nil, err
		}
		changes[modelRef(configID)] = v
	}
	previousAll := *c.Configs.ALLConfig
	previousEnabled := c.Configs.EnablesConfig
	*c.Configs.ALLConfig = append(previousAll, entry)
	c.Configs.EnablesConfig = &(*c.Configs.ALLConfig)[len(*c.Configs.ALLConfig)-1]
	if err := c.withCredentials(changes, false, c.saveConfig); err != nil {
		*c.Configs.ALLConfig = previousAll
		c.Configs.EnablesConfig = previousEnabled
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}
	return c.Configs.snapshot(), nil
}

// RefreshModelConfig 更新已有配置探测到的聊天地址和模型列表。
func (c *Core) RefreshModelConfig(index int, serverAddress string, models []apiImpl.Model) (*Configs, error) {
	return c.UpdateModelCredentials(index, serverAddress, models, "", false)
}

// A blank replacement keeps the saved key; clearing it is an explicit action.
func (c *Core) UpdateModelCredentials(index int, serverAddress string, models []apiImpl.Model, replacement string, clearKey bool) (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	entry, err := c.configAt(index)
	if err != nil {
		return nil, err
	}
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return nil, errors.New("API 地址不能为空")
	}
	cleanModels := make([]apiImpl.Model, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		if model.Name == "" {
			model.Name = model.ID
		}
		cleanModels = append(cleanModels, model)
	}
	if len(cleanModels) == 0 {
		return nil, errors.New("探测结果中没有可用模型，原配置未更改")
	}

	changes := map[string]credentials.Entry{}
	hadKey := entry.HasAPIKey
	if clearKey && replacement != "" {
		return nil, errors.New("不能同时替换和清除密钥")
	}
	if clearKey {
		changes[modelRef(entry.ID)] = credentials.Entry{}
		entry.HasAPIKey = false
	} else if replacement != "" {
		v, err := modelSecret(serverAddress, replacement)
		if err != nil {
			return nil, err
		}
		changes[modelRef(entry.ID)] = v
		entry.HasAPIKey = true
	} else if hadKey {
		if _, err := c.providerKey(entry, serverAddress); err != nil {
			return nil, err
		}
	}
	previousAddress := entry.ServerAddress
	previousModels := entry.Model
	previousEnabledModel := entry.EnabledModel
	entry.ServerAddress = serverAddress
	entry.Model = cleanModels
	if !slices.ContainsFunc(cleanModels, func(model apiImpl.Model) bool {
		return model.ID == entry.EnabledModel
	}) {
		entry.EnabledModel = cleanModels[0].ID
	}
	if err := c.withCredentials(changes, false, c.saveConfig); err != nil {
		entry.HasAPIKey = hadKey
		entry.ServerAddress = previousAddress
		entry.Model = previousModels
		entry.EnabledModel = previousEnabledModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}
	return c.Configs.snapshot(), nil
}

// RenameConfig 供前端调用：修改第 index 个配置的名称并写回配置文件。
// 名称首尾空白会被去掉；留空表示不命名，前端展示时回退为服务地址。
func (c *Core) RenameConfig(index int, name string) (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	entry, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousName := entry.Name
	entry.Name = strings.TrimSpace(name)

	if err := c.saveConfig(); err != nil {
		entry.Name = previousName
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs.snapshot(), nil
}

// DeleteConfig 供前端调用：删除第 index 个配置并写回配置文件。
// 若删除的是当前启用的配置，启用状态一并清空。
func (c *Core) DeleteConfig(index int) (*Configs, error) {
	if c == nil {
		return nil, errors.New("宿主尚未初始化")
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	if c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("配置尚未加载")
	}

	all := *c.Configs.ALLConfig
	if index < 0 || index >= len(all) {
		return nil, fmt.Errorf("配置下标越界：%d（共 %d 项）", index, len(all))
	}

	previousAll := all
	previousEnables := c.Configs.EnablesConfig

	removed := &all[index]
	*c.Configs.ALLConfig = append(all[:index:index], all[index+1:]...)
	if c.Configs.EnablesConfig == removed {
		c.Configs.EnablesConfig = nil
	} else {
		c.Configs.resolveEnabled()
	}

	changes := map[string]credentials.Entry{}
	if removed.HasAPIKey {
		changes[modelRef(removed.ID)] = credentials.Entry{}
	}
	if err := c.withCredentials(changes, false, c.saveConfig); err != nil {
		*c.Configs.ALLConfig = previousAll
		c.Configs.EnablesConfig = previousEnables
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs.snapshot(), nil
}

type chatIndex struct {
	Version int                `json:"version"`
	Chats   []chat.ChatSummary `json:"chats"`
}

const chatIndexVersion = 1

func (c *Core) chatDirectory() (string, error) {
	if c == nil || c.Paths == nil || c.FS == nil {
		return "", errors.New("会话存储尚未初始化")
	}
	directory := filepath.Join(c.Paths.Data, "chats")
	if err := c.FS.CreateDirectory(directory, 0o700); err != nil {
		return "", fmt.Errorf("创建会话目录失败：%w", err)
	}
	return directory, nil
}

func validChatID(id string) bool {
	return identity.Valid(id, identity.Chat)
}

func (c *Core) readChatIndexLocked(directory string) (chatIndex, error) {
	data, err := c.readConversationFile(filepath.Join(directory, "index.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return chatIndex{Version: chatIndexVersion, Chats: []chat.ChatSummary{}}, nil
	}
	if err != nil {
		return chatIndex{}, err
	}
	var index chatIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return chatIndex{}, fmt.Errorf("解析会话索引失败：%w", err)
	}
	if index.Chats == nil {
		index.Chats = []chat.ChatSummary{}
	}
	// 旧索引没有工作区字段，从会话文件补齐一次，之后只需读取摘要。
	if index.Version < chatIndexVersion {
		for i := range index.Chats {
			id := index.Chats[i].ID
			if !validChatID(id) {
				return chatIndex{}, errors.New("会话索引包含无效 ID")
			}
			data, err := c.readConversationFile(filepath.Join(directory, id+".json"))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return chatIndex{}, fmt.Errorf("读取会话工作区失败：%w", err)
			}
			var stored struct {
				ID          string `json:"id"`
				WorkspaceID string `json:"workspaceId"`
			}
			if err := json.Unmarshal(data, &stored); err != nil {
				return chatIndex{}, fmt.Errorf("解析会话工作区失败：%w", err)
			}
			if stored.ID != id {
				return chatIndex{}, errors.New("会话文件 ID 不匹配")
			}
			index.Chats[i].WorkspaceID = stored.WorkspaceID
		}
		index.Version = chatIndexVersion
		if err := c.writeJSONLocked(filepath.Join(directory, "index.json"), index); err != nil {
			return chatIndex{}, fmt.Errorf("更新会话索引失败：%w", err)
		}
	}
	return index, nil
}

func (c *Core) writeJSONLocked(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if c.profileRoot != "" {
		if scope := conversationScope(c.Paths.Data, path); scope != "" {
			data, err = c.vault.ChatBytes(scope, data, true)
			if err != nil {
				return err
			}
		}
	}
	tmp := path + ".tmp"
	if err := c.FS.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := c.FS.MoveFile(tmp, path); err != nil {
		_ = c.FS.DeleteFile(tmp)
		return err
	}
	return nil
}

func (c *Core) readChatLocked(directory, id string) (*chat.ChatTask, error) {
	if !validChatID(id) {
		return nil, errors.New("无效会话 ID")
	}
	data, err := c.readConversationFile(filepath.Join(directory, id+".json"))
	if err != nil {
		return nil, err
	}
	var session chat.ChatTask
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("解析会话失败：%w", err)
	}
	if session.ID != id {
		return nil, errors.New("会话文件 ID 不匹配")
	}
	if session.Tree == nil {
		session.Tree = chatstruct.CreateTree(session.Title)
	}
	fillToolDisplayNames(session.Tree)
	if err := c.cleanChatMemoryIDsLocked(directory, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// 旧会话没有保存展示名，按工具 ID 补齐所有分支；已有名称保留。
func fillToolDisplayNames(tree *chatstruct.ChatTree) {
	if tree == nil || tree.Root == nil {
		return
	}
	pending := append([]*chatstruct.ChatNode(nil), tree.Root.Next...)
	seen := make(map[*chatstruct.ChatNode]bool)
	for len(pending) > 0 {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if node == nil || seen[node] {
			continue
		}
		seen[node] = true
		for i := range node.Child {
			for j := range node.Child[i].ToolCalls {
				call := &node.Child[i].ToolCalls[j]
				if call.DisplayName == "" {
					call.DisplayName = agenttool.DisplayName(call.Name)
				}
				if tool := agenttool.Tools[call.Name]; tool != nil && tool.Meta() != nil {
					if call.DisplayType == "" {
						call.DisplayType = tool.Meta().DisplayType
					}
					if call.DisplayContent == "" {
						call.DisplayContent = tool.Meta().DisplayContent
					}
				}
			}
		}
		pending = append(pending, node.Next...)
	}
}

func (c *Core) saveChatLocked(directory string, session *chat.ChatTask) error {
	if err := c.writeJSONLocked(filepath.Join(directory, session.ID+".json"), session); err != nil {
		return fmt.Errorf("保存会话失败：%w", err)
	}
	index, err := c.readChatIndexLocked(directory)
	if err != nil {
		return fmt.Errorf("读取会话索引失败：%w", err)
	}
	summary := chat.ChatSummary{
		ID: session.ID, Title: session.Title, ConfigID: session.ConfigID,
		Model: session.Model, WorkspaceID: session.WorkspaceID, UpdatedAt: session.UpdatedAt,
	}
	found := false
	for i := range index.Chats {
		if index.Chats[i].ID == session.ID {
			index.Chats[i] = summary
			found = true
			break
		}
	}
	if !found {
		index.Chats = append(index.Chats, summary)
	}
	return c.writeJSONLocked(filepath.Join(directory, "index.json"), index)
}

// Return a snapshot so an in-flight chat keeps the exact address checked for its key.
func (c *Core) configByID(id string) (*Config, error) {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	if c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("模型配置尚未加载")
	}
	for i := range *c.Configs.ALLConfig {
		if (*c.Configs.ALLConfig)[i].ID == id {
			return (*c.Configs.ALLConfig)[i].snapshot(), nil
		}
	}
	return nil, fmt.Errorf("会话引用的配置 %q 不存在，请重新选择配置", id)
}

func (c *Core) defaultProvider() (*Config, error) {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	if c.Configs == nil {
		return nil, errors.New("模型配置尚未加载")
	}
	if c.Configs.EnablesConfig == nil {
		return nil, errors.New("尚未选择配置（提供商），请先选择配置")
	}
	return c.Configs.EnablesConfig.snapshot(), nil
}

func (c *Core) ListChats() ([]chat.ChatSummary, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	index, err := c.readChatIndexLocked(directory)
	if err != nil {
		return nil, err
	}
	sort.Slice(index.Chats, func(i, j int) bool {
		return index.Chats[i].UpdatedAt.After(index.Chats[j].UpdatedAt)
	})
	return index.Chats, nil
}

// ListPersonas 返回可用的人设列表（当前全部是代码内置的）。
func (c *Core) ListPersonas() []persona.Persona {
	return persona.List()
}

func (c *Core) CreateChat(configID, model, title, personaID string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	var cfg *Config
	var err error
	if configID == "" {
		cfg, err = c.defaultProvider()
	} else {
		cfg, err = c.configByID(configID)
	}
	if err != nil {
		return nil, err
	}
	if model == "" {
		model = cfg.EnabledModel
	}
	if !slices.ContainsFunc(cfg.Model, func(item apiImpl.Model) bool { return item.ID == model }) {
		return nil, fmt.Errorf("模型 %q 不属于所选配置", model)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新建对话"
	}
	now := time.Now().UTC()
	tree := chatstruct.CreateTree(title)
	// 选择初始人设，所有分支共享；找不到就回落到默认。
	tree.Root.PersonaID = persona.Resolve(personaID).ID
	chatID, err := identity.New(identity.Chat)
	if err != nil {
		return nil, err
	}
	session := chat.NewChatTask(chatID, title, cfg.ID, model, tree)
	session.MemoryLibraryIDs, err = c.newChatMemoryIDs()
	if err != nil {
		return nil, err
	}
	session.CreatedAt = now
	session.UpdatedAt = now
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	workspace, err := c.chatWorkspaceLocked(chatID, "")
	if err != nil {
		return nil, err
	}
	if err := c.saveChatLocked(directory, session); err != nil {
		_ = c.FS.DeleteFile(filepath.Join(directory, session.ID+".json"))
		_ = c.FS.DeleteFile(workspace.Path) // 刚分配的空目录；不会递归删除文件。
		return nil, err
	}
	return session, nil
}

func (c *Core) GetChat(id string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	return c.readChatLocked(directory, id)
}

func (c *Core) RenameChat(id, title string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法修改")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("对话标题不能为空")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	session.Title = title
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Core) DeleteChat(id string) error {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return errors.New("对话正在生成中，暂时无法删除")
	}
	delete(chatServer.ChatList, id)
	directory, err := c.chatDirectory()
	if err != nil {
		return err
	}
	if _, err := c.readChatLocked(directory, id); err != nil {
		return err
	}
	if err := c.deleteChatFiles(directory, id); err != nil {
		return err
	}
	index, err := c.readChatIndexLocked(directory)
	if err != nil {
		return err
	}
	filtered := index.Chats[:0]
	for _, item := range index.Chats {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	index.Chats = filtered
	return c.writeJSONLocked(filepath.Join(directory, "index.json"), index)
}

// SetChatPersona 切换当前对话的人设，后续请求使用新的角色提示词。
func (c *Core) SetChatPersona(id, personaID string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法切换人设")
	}
	selected := persona.Find(personaID)
	if selected == nil {
		return nil, fmt.Errorf("人设 %q 不存在", personaID)
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	if session.Tree.Root == nil {
		session.Tree.Root = &chatstruct.TreeRoot{Name: session.Title}
	}
	if session.Tree.Root.PersonaID == selected.ID {
		return session, nil
	}
	session.Tree.Root.PersonaID = selected.ID
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Core) UpdateChatModel(id, configID, model string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法切换模型")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	cfg, err := c.configByID(configID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(cfg.Model, func(item apiImpl.Model) bool { return item.ID == model }) {
		return nil, fmt.Errorf("模型 %q 不属于所选配置", model)
	}
	session.ConfigID = configID
	session.Model = model
	delete(chatServer.ChatList, id)
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Core) SetChatBranch(id, parentID string, choice int) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.toolMu.Lock()
	defer chatServer.toolMu.Unlock()
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法切换分支")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	before, err := session.Tree.CurrentPath()
	if err != nil {
		return nil, err
	}
	previousChoice := session.Tree.Root.Choice
	previousUpdatedAt := session.UpdatedAt
	parent := session.Tree.FindNode(parentID)
	if parentID != "" && parent != nil {
		previousChoice = parent.Choice
	}
	if err := session.Tree.SelectBranch(parentID, choice); err != nil {
		return nil, err
	}
	after, err := session.Tree.CurrentPath()
	if err == nil {
		err = c.transitionToolPaths(before, after)
	}
	if err != nil {
		_ = session.Tree.SelectBranch(parentID, previousChoice)
		return nil, err
	}
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		_ = session.Tree.SelectBranch(parentID, previousChoice)
		session.UpdatedAt = previousUpdatedAt
		return nil, errors.Join(err, c.transitionToolPaths(after, before), c.saveChatLocked(directory, session))
	}
	return session, nil
}

func (c *Core) GetChatModelContextWindow(id string) (int, error) {
	task, err := c.GetChat(id)
	if err != nil {
		return 0, err
	}
	config, err := c.configByID(task.ConfigID)
	if err != nil {
		return 0, err
	}
	for _, model := range config.Model {
		if model.ID == task.Model {
			if model.MaxContext <= 0 {
				return 0, fmt.Errorf("模型 %q 未保存有效的上下文窗口数据", task.Model)
			}
			return model.MaxContext, nil
		}
	}
	return 0, fmt.Errorf("模型列表中未找到 %q", task.Model)
}

// normalizeReasoningEffort 把非法值收敛成默认的 "high"（中），避免脏数据写进会话文件。
func normalizeReasoningEffort(effort string) string {
	switch effort {
	case "none", "low", "high", "max":
		return effort
	default:
		return "high"
	}
}

// SetChatReasoningEffort 把思考强度保存到对话里，切换或重开对话时会读回。
func (c *Core) SetChatReasoningEffort(id, effort string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法修改思考强度")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	session.ReasoningEffort = normalizeReasoningEffort(effort)
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

// ===== 对话级工具启用配置 =====
// 最小启停单位是 ToolKit，不是单个 Tool。

type ToolDescriptor struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Parameters     map[string]any `json:"parameters"`
	DisplayType    string         `json:"displayType,omitempty"`
	DisplayContent string         `json:"displayContent,omitempty"`
}

type ToolKitDescriptor struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	DefaultEnabled bool             `json:"defaultEnabled"`
	Tools          []ToolDescriptor `json:"tools"`
}

type ToolSetDescriptor struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	DefaultEnabled bool                `json:"defaultEnabled"`
	Kits           []ToolKitDescriptor `json:"kits"`
}

// ListToolSets 返回工具集、模块及工具的完整参数声明，供前端渲染开关和详情。
func (c *Core) ListToolSets() []ToolSetDescriptor {
	out := make([]ToolSetDescriptor, 0, len(agenttool.ToolSets))
	for setID, set := range agenttool.ToolSets {
		meta := set.Meta()
		item := ToolSetDescriptor{
			ID: setID, Name: meta.Name, Description: meta.Description,
			DefaultEnabled: meta.Enabled, Kits: []ToolKitDescriptor{},
		}
		for kitID, kit := range meta.ToolKits {
			kitMeta := kit.Meta()
			tools := make([]ToolDescriptor, 0, len(kitMeta.Tools))
			for _, tool := range kit.List() {
				if tool == nil {
					continue
				}
				if tm := tool.Meta(); tm != nil {
					tools = append(tools, ToolDescriptor{
						ID: tm.ID, Name: tm.Name, Description: tm.Description,
						Parameters:  tm.ToType().Parameters,
						DisplayType: tm.DisplayType, DisplayContent: tm.DisplayContent,
					})
				}
			}
			sort.Slice(tools, func(i, j int) bool { return tools[i].ID < tools[j].ID })
			item.Kits = append(item.Kits, ToolKitDescriptor{
				ID: kitID, Name: kitMeta.Name, Description: kitMeta.Description,
				DefaultEnabled: meta.Enabled && kitMeta.Enabled,
				Tools:          tools,
			})
		}
		sort.Slice(item.Kits, func(i, j int) bool { return item.Kits[i].ID < item.Kits[j].ID })
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SetChatToolConfig 把对话级覆盖写入会话文件。raw 是 ChatToolConfig 的 JSON；
// 传 "{}" 表示清空覆盖，全部回退到全局默认。
func (c *Core) SetChatToolConfig(id, raw string) (*chat.ChatTask, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	if chatServer.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法修改工具配置")
	}
	var cfg agenttool.ChatToolConfig
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, err
		}
	}
	cfg.Normalize()
	if cfg.Permission < 0 || cfg.Permission > agenttool.PermissionFullAccess {
		return nil, errors.New("无效的工具权限配置")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	previousPermission := c.toolPermission(session.ToolConfig)
	if c.toolPermission(&cfg) < previousPermission {
		cfg.AllowedTools = nil
	}
	for toolID, grant := range cfg.AllowedTools {
		if agenttool.Tools[toolID] == nil || toolID == agenttool.RequestToolExecutionID {
			return nil, errors.New("授权记录包含无效工具")
		}
		if grant.Scope != agenttool.GrantTool && grant.Scope != agenttool.GrantWorkspace {
			return nil, errors.New("授权作用范围无效")
		}
		if grant.Scope == agenttool.GrantWorkspace {
			if grant.WorkspaceID == "" {
				return nil, errors.New("工作区授权缺少工作区 ID")
			}
			if grant.WorkspaceID == chatInternalWorkspaceID(session.ID) {
				continue // 本对话的内部工作区不登记在公共工作区列表中。
			}
			if _, err := c.workspaceByIDLocked(grant.WorkspaceID); err != nil {
				return nil, err
			}
		} else if grant.WorkspaceID != "" {
			return nil, errors.New("工具授权不能附带工作区限制")
		}
	}
	if cfg.Sets == nil && cfg.Permission == 0 && cfg.StrictMode == nil && len(cfg.AllowedTools) == 0 {
		session.ToolConfig = nil
	} else {
		session.ToolConfig = &cfg
	}
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

// enabledTools 按「对话覆盖 → 全局默认 → 代码默认」解析每个 ToolKit，
// 把启用 kit 下的工具转成请求可直接携带的 apiImpl.Tool。
func (c *Core) enabledTools(session *chat.ChatTask) []apiImpl.Tool {
	if session == nil {
		return nil
	}
	defaults := c.defaultTools()
	var enabled []apiImpl.Tool
	for setID, set := range agenttool.ToolSets {
		meta := set.Meta()
		for kitID, kit := range meta.ToolKits {
			on, explicit := session.ToolConfig.Kit(setID, kitID)
			if !explicit {
				on, explicit = defaults.Kit(setID, kitID)
			}
			if !explicit {
				on = meta.Enabled && kit.Meta().Enabled
			}
			if !on {
				continue
			}
			for _, tool := range kit.List() {
				tm := tool.Meta()
				if tm == nil || !tm.Enabled {
					continue
				}
				if tm.ID == agenttool.RequestToolExecutionID && !c.strictToolMode(session) {
					continue
				}
				if converted, ok := agenttool.ToolList[tm.ID]; ok {
					enabled = append(enabled, converted)
				}
			}
		}
	}
	if c.strictToolMode(session) && !slices.ContainsFunc(enabled, func(tool apiImpl.Tool) bool { return tool.Name == agenttool.RequestToolExecutionID }) {
		if request, exists := agenttool.ToolList[agenttool.RequestToolExecutionID]; exists && agenttool.Tools[agenttool.RequestToolExecutionID].Meta().Enabled {
			enabled = append(enabled, request)
		}
	}
	// 注册表用 map 保存；声明排序固定，避免相同配置的请求前缀随机变化。
	slices.SortFunc(enabled, func(left, right apiImpl.Tool) int { return strings.Compare(left.Name, right.Name) })
	return enabled
}

func (c *Core) SendChatMessage(id, text, reasoningEffort string) error {
	return c.SendChatMessageWithAttachments(id, text, reasoningEffort, nil)
}
func (c *Core) SendChatMessageWithAttachments(id, text, reasoningEffort string, files []AttachmentUpload) error {
	text = strings.TrimSpace(text)
	if text == "" && len(files) == 0 {
		return errors.New("消息不能为空")
	}
	session, config, directory, err := c.beginChatSend(id)
	if err != nil {
		return err
	}
	defer c.endChatSend(id)

	path, err := session.Tree.CurrentPath()
	if err != nil {
		return err
	}
	modelIndex := slices.IndexFunc(config.Model, func(m apiImpl.Model) bool { return m.ID == session.Model })
	if modelIndex < 0 {
		return errors.New("模型不在配置中")
	}
	attachments, err := prepareUploads(files, config.Model[modelIndex])
	if err != nil {
		return err
	}
	message := apiImpl.ChatMessage{Role: "user", Content: text}
	for _, item := range attachments {
		if item.ref.IsImage() {
			if err = c.saveImage(directory, id, item.ref, item.data); err != nil {
				for _, prev := range message.Attachments {
					if rel, e := imageFile(id, prev); e == nil {
						_ = c.FS.DeleteFileInRoot(directory, rel)
					}
				}
				return err
			}
		}
		message.Attachments = append(message.Attachments, item.ref)
	}
	userNode := chatstruct.CreateChatNode(message)
	if len(path) == 0 {
		session.Tree.Root.Add(userNode)
	} else if err := session.Tree.AddNext(path[len(path)-1].ID, userNode); err != nil {
		return err
	}
	if session.Title == "新建对话" {
		title := text
		if title == "" {
			title = message.Attachments[0].Name
		}
		session.Title = titleFromMessage(title)
	}
	replyNode := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(userNode.ID, replyNode); err != nil {
		return err
	}
	return c.runChatNode(id, session, config, directory, replyNode, reasoningEffort)
}

func (c *Core) RegenerateChatMessage(id, nodeID, reasoningEffort string) error {
	session, config, directory, err := c.beginChatSend(id)
	if err != nil {
		return err
	}
	defer c.endChatSend(id)

	parent, found := session.Tree.ParentOf(nodeID)
	if !found || parent == nil {
		return errors.New("只能重新生成有父节点的回复")
	}
	target := session.Tree.FindNode(nodeID)
	if target == nil || !hasAssistantMessage(target) {
		return errors.New("只能重新生成助手回复")
	}
	path, err := session.Tree.CurrentPath()
	if err != nil {
		return err
	}
	parentOnPath := false
	for _, node := range path {
		if node == parent {
			parentOnPath = true
			break
		}
	}
	if !parentOnPath {
		return errors.New("只能重新生成当前分支上的回复")
	}
	newReply := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(parent.ID, newReply); err != nil {
		return err
	}
	return c.runChatNode(id, session, config, directory, newReply, reasoningEffort)
}

// EditChatMessage 编辑当前分支上的一条用户消息。
// 不改动原节点，而是在它的父节点下挂一条新用户消息形成新分支，原分支仍可切回。
func (c *Core) EditChatMessage(id, nodeID, text, reasoningEffort string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("消息不能为空")
	}
	session, config, directory, err := c.beginChatSend(id)
	if err != nil {
		return err
	}
	defer c.endChatSend(id)

	// ParentOf 对抽象根的直接子节点返回 (nil, true)，nil 表示挂在 Root 下
	parent, found := session.Tree.ParentOf(nodeID)
	if !found {
		return errors.New("找不到这条消息的父节点")
	}
	target := session.Tree.FindNode(nodeID)
	if target == nil || !hasUserMessage(target) {
		return errors.New("只能编辑用户消息")
	}
	if parent != nil {
		path, err := session.Tree.CurrentPath()
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(path, func(node *chatstruct.ChatNode) bool { return node == parent }) {
			return errors.New("只能编辑当前分支上的消息")
		}
	}

	parentID := ""
	if parent != nil {
		parentID = parent.ID
	}
	message := apiImpl.ChatMessage{Role: "user", Content: text}
	for _, old := range target.Child {
		if old.Role == "user" {
			message.Attachments = slices.Clone(old.Attachments)
			break
		}
	}
	userNode := chatstruct.CreateChatNode(message)
	if err := session.Tree.AddNext(parentID, userNode); err != nil {
		return err
	}
	replyNode := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(userNode.ID, replyNode); err != nil {
		return err
	}
	return c.runChatNode(id, session, config, directory, replyNode, reasoningEffort)
}

func hasUserMessage(node *chatstruct.ChatNode) bool {
	for _, message := range node.Child {
		if message.Role == "user" {
			return true
		}
	}
	return false
}

func hasAssistantMessage(node *chatstruct.ChatNode) bool {
	if node.Stopped {
		return true // 用户在首个分片前停止时，空的回复节点也允许重新生成。
	}
	for _, message := range node.Child {
		if message.Role == "assistant" {
			return true
		}
	}
	return false
}

func titleFromMessage(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 40 {
		return string(runes[:40]) + "…"
	}
	return text
}

func (c *Core) beginChatSend(id string) (*chat.ChatTask, *Config, string, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return nil, nil, "", serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	parent, err := chatServer.contextLocked()
	if err != nil {
		return nil, nil, "", err
	}
	chatServer.initLocked()
	if chatServer.sending[id] != nil {
		return nil, nil, "", errors.New("此对话正在生成中")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, nil, "", err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, nil, "", err
	}
	config, err := c.configByID(session.ConfigID)
	if err != nil {
		return nil, nil, "", err
	}
	if !slices.ContainsFunc(config.Model, func(item apiImpl.Model) bool { return item.ID == session.Model }) {
		return nil, nil, "", fmt.Errorf("会话模型 %q 已不在配置中，请重新选择模型", session.Model)
	}
	ctx, cancel := context.WithCancel(parent)
	chatServer.sending[id] = &chatSend{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	return session, config, directory, nil
}

func (c *Core) endChatSend(id string) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return
	}

	chatServer.mu.Lock()
	if run := chatServer.sending[id]; run != nil {
		run.cancel()
		close(run.done)
	}
	delete(chatServer.sending, id)
	chatServer.mu.Unlock()
}

// 一轮工具调用后模型会带着结果重新请求，限制轮数防止模型无限调用。
const maxToolRounds = 10

var errToolDisabled = errors.New("此工具被禁用")

func (c *Core) runChatNode(id string, session *chat.ChatTask, config *Config, directory string, reply *chatstruct.ChatNode, reasoningEffort string, executionOptions ...*chatExecutionOptions) (outErr error) {
	options := firstExecutionOptions(executionOptions)
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return serviceErr
	}
	chatServer.mu.Lock()
	run := chatServer.sending[id]
	chatServer.mu.Unlock()
	if run == nil {
		return errors.New("对话生成任务不存在")
	}
	ctx := run.ctx
	if err := ctx.Err(); err != nil {
		return err
	}
	modelIndex := slices.IndexFunc(config.Model, func(model apiImpl.Model) bool { return model.ID == session.Model })
	if modelIndex < 0 {
		return fmt.Errorf("模型列表中未找到 %q", session.Model)
	}
	model := config.Model[modelIndex]
	vision, err := newChatVision(c, directory, session, reply, model)
	if err != nil {
		return err
	}
	toolContext, memoryContext, err := c.chatExecutionContext(ctx, session, options)
	if err != nil {
		return err
	}
	approvals, err := c.Servers.Get[*ToolApprovalServer](ToolApprovalServerName)
	if err != nil {
		return err
	}
	// 每次生成都把本次使用的思考强度写进会话，随后 saveChatLocked 会一并落盘。
	session.ReasoningEffort = normalizeReasoningEffort(reasoningEffort)
	reply.ReasoningEffort = session.ReasoningEffort
	session.UpdatedAt = time.Now().UTC()
	if options == nil {
		// 校验完成后才切换文件状态，并将 Choice 和新分支一起保存。
		err = c.saveChatWithToolTransition(directory, session)
	} else {
		chatServer.mu.Lock()
		err = c.saveExecutionChatLocked(directory, session, options)
		chatServer.mu.Unlock()
	}
	if err != nil {
		return err
	}

	if options == nil {
		chatServer.mu.Lock()
		run.nodeID = reply.ID
		chatServer.mu.Unlock()
		defer func() {
			chatServer.mu.Lock()
			stopped := run.userStopped
			run.nodeID = ""
			chatServer.mu.Unlock()
			if !stopped {
				return
			}
			reply.Stopped = true
			// 首个分片前停止时只保存节点标记；已经生成的文本、思考和工具调用仍然保留。
			reply.Child = slices.DeleteFunc(reply.Child, func(m apiImpl.ChatMessage) bool {
				return m.Role == "assistant" && m.Content == "" && m.Reasoning == "" && len(m.ToolCalls) == 0
			})
			// 补全停止时尚未执行的工具结果，使下次请求的调用和结果仍一一对应。
			completed := map[string]bool{}
			for _, m := range reply.Child {
				if m.Role == "tool" {
					completed[m.ToolCallID] = true
				}
			}
			const skipped = "本轮生成已由用户停止，未执行此工具调用。"
			for _, m := range reply.Child {
				for _, call := range m.ToolCalls {
					if !completed[call.ID] {
						reply.Child = append(reply.Child, apiImpl.ChatMessage{Role: "tool", ToolCallID: call.ID, Content: skipped, ToolError: skipped})
						completed[call.ID] = true
					}
				}
			}
			if errors.Is(outErr, context.Canceled) {
				outErr = nil
			}
			outErr = errors.Join(outErr, c.saveReplyLocked(directory, session, options))
		}()
		EmitJSON(EventChatGeneration, map[string]string{"chatId": id, "nodeId": reply.ID})
	}
	if err := c.chatExecutionClient(session, config); err != nil {
		return err
	}
	tools := c.enabledTools(session)
	request := &apiImpl.ChatRequest{Model: model.ID, Thinking: session.ReasoningEffort, Tools: tools}
	toolRun := &toolExecutionRun{
		core: c, vision: vision, approvals: approvals, session: session, generation: run,
		directory: directory, nodeID: reply.ID, context: toolContext, enabled: tools,
		strict: c.strictToolMode(session), blocked: make(map[string]apiImpl.ToolCall), seen: make(map[string]bool),
		options: options,
	}
	// 压缩只影响请求历史；produced 始终完整保存本节点实际产出的消息。
	prefix := chatExecutionPrefix(session, toolContext, memoryContext, options, toolRun.strict)
	policyIndex := len(prefix) - 1
	produced := make([]apiImpl.ChatMessage, 0, 4)
	reply.Child = produced
	if err := c.restoreCompactionReads(toolRun, model); err != nil {
		return err
	}
	history, err := chatExecutionHistory(session, model, prefix)
	if err != nil {
		return err
	}

	keep := compactionKeepBudget(model, prefix, request.Tools)
	policy := c.chatDefaults().Compaction
	if session.CompactionMode != nil {
		policy.CompactionMode = *session.CompactionMode
	}
	for round := 0; round <= maxToolRounds; round++ {
		if err := ctx.Err(); err != nil {
			c.saveReplyLocked(directory, session, options)
			return err
		}
		assistant := apiImpl.ChatMessage{Role: "assistant"}
		prefix[policyIndex].Content = toolPolicyPrompt(toolRun.strict, toolContext, session.ToolConfig)
		history[policyIndex] = prefix[policyIndex]
		request.Messages = vision.messages(history)
		// 只在完整工具轮之后检查预算，避免拆散调用和结果或重放已执行工具。
		used := apiImpl.RequestTokens(*request)
		compressed, err := c.maybeCompactChat(toolRun, reply, model, policy, keep, used)
		if err != nil {
			return errors.Join(err, c.saveReplyLocked(directory, session, options))
		}
		if compressed {
			history, err = chatExecutionHistory(session, model, prefix)
			if err != nil {
				return err
			}
			request.Messages = vision.messages(history)
			used = apiImpl.RequestTokens(*request)
		}
		if model.MaxContext > 0 && used+compactionOutputReserve(model) > model.MaxContext {
			return errors.New("当前上下文及输出预留超过模型窗口，原始对话已保留；请立即压缩上下文、缩短过大的单条消息或选择更大的窗口")
		}
		var requestUsage *apiImpl.TokenUsage
		streamErr := session.Client.StreamChat(ctx, *request, &apiImpl.ChatCompletionAPI{}, func(delta apiImpl.Delta) {
			// 单独的 [DONE] 只是传输结束；收到模型输出或输入用量后才显示读图回执。
			if len(vision.pending) > 0 && (delta.Content != "" || delta.Reasoning != "" || len(delta.ToolCalls) > 0 || (delta.Usage != nil && delta.Usage.PromptTokens > 0)) {
				vision.received(reply)
			}
			if delta.Content != "" {
				assistant.Content += delta.Content
			}
			if delta.Reasoning != "" {
				assistant.Reasoning += delta.Reasoning
			}
			if len(delta.ToolCalls) > 0 {
				for i := range delta.ToolCalls {
					delta.ToolCalls[i].DisplayName = agenttool.DisplayName(delta.ToolCalls[i].Name)
					if tool := agenttool.Tools[delta.ToolCalls[i].Name]; tool != nil && tool.Meta() != nil {
						delta.ToolCalls[i].DisplayType = tool.Meta().DisplayType
						delta.ToolCalls[i].DisplayContent = tool.Meta().DisplayContent
					}
				}
				assistant.ToolCalls = append(assistant.ToolCalls, delta.ToolCalls...)
			}
			if delta.Usage != nil {
				usage := *delta.Usage
				reply.Usage = &usage
				requestUsage = &usage
			}
			if delta.Content != "" || delta.Reasoning != "" || delta.Usage != nil {
				EmitJSON(EventChatDelta, map[string]any{
					"chatId": id, "nodeId": reply.ID, "content": delta.Content,
					"reasoning": delta.Reasoning, "usage": delta.Usage,
				})
			}
		})
		// 流可能多次上报用量快照，只记录最后一份；不持有 ChatServer.mu。
		if options != nil && options.recordUsage != nil {
			options.recordUsage(requestUsage)
		}
		assistant.Tokens = apiImpl.MessageTokens(assistant)
		if requestUsage != nil && requestUsage.CompletionTokens > 0 {
			assistant.Tokens = requestUsage.CompletionTokens + 4
		}
		produced = append(produced, assistant)
		reply.Child = produced
		if streamErr == nil && len(vision.pending) > 0 {
			streamErr = errors.New("模型服务没有返回有效的图片请求响应")
		}
		if streamErr != nil {
			c.saveReplyLocked(directory, session, options)
			return streamErr
		}

		// 没有工具调用说明这一轮就是最终回复
		if len(assistant.ToolCalls) == 0 {
			break
		}

		history = append(history, assistant)
		for callIndex := range assistant.ToolCalls {
			call := &assistant.ToolCalls[callIndex]
			if err := ctx.Err(); err != nil {
				c.saveReplyLocked(directory, session, options)
				return err
			}
			payload := map[string]string{
				"chatId": id, "nodeId": reply.ID, "toolId": call.Name, "callId": call.ID,
				"name": call.DisplayName, "event": "start",
				"displayType": call.DisplayType, "displayContent": call.DisplayContent,
			}
			if call.DisplayContent == "arguments" || strings.HasPrefix(call.DisplayContent, "arguments.") {
				payload["content"] = call.Arguments
			}
			EmitJSON(EventChatTool, payload)
			output, callErr := toolRun.execute(call)
			if callErr == nil && call.Name == agenttool.ReadImageID {
				for _, p := range vision.pending {
					if p.callID == call.ID {
						a := p.ref
						call.Image = &a
						break
					}
				}
			}
			toolError := ""
			if callErr != nil {
				payload["event"] = "fail"
				toolError = callErr.Error()
				payload["error"] = toolError
				// 工具失败不能中断整轮，回一条 tool 消息让模型自己处理
				if errors.Is(callErr, errToolDisabled) {
					output = callErr.Error()
				} else {
					output = "工具执行失败：" + callErr.Error()
				}
			} else {
				payload["event"] = "done"
			}
			if call.DisplayContent == "result" {
				payload["content"] = output
			}
			if call.DisplayType == "question" {
				payload["answer"] = output
			}
			if call.Approval != nil {
				body, _ := json.Marshal(call.Approval)
				payload["approval"] = string(body)
			}
			EmitJSON(EventChatTool, payload)
			toolMsg := apiImpl.ChatMessage{Role: "tool", ToolCallID: call.ID, Content: output, ToolError: toolError}
			toolMsg.Tokens = apiImpl.MessageTokens(toolMsg)
			history = append(history, toolMsg)
			produced = append(produced, toolMsg)
			reply.Child = produced
		}

		session.UpdatedAt = time.Now().UTC()
		if err := c.saveReplyLocked(directory, session, options); err != nil {
			return err
		}
	}

	return c.saveReplyLocked(directory, session, options)
}

func (c *Core) saveReplyLocked(directory string, session *chat.ChatTask, options ...*chatExecutionOptions) error {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return serviceErr
	}

	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	return c.saveExecutionChatLocked(directory, session, firstExecutionOptions(options))
}
