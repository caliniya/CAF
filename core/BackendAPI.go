package Core

import (
	"context"
	"encoding/hex"
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
)

func (c *Core) StartLogin() (int, error) {
	if c == nil || c.QQPlatform == nil {
		return 0, errors.New("QQ 平台尚未初始化")
	}

	return c.QQPlatform.StartLogin(), nil
}

// SelectDirectory 供前端调用平台文件夹选择器；取消时返回空字符串。
func (c *Core) SelectDirectory(defaultDirectory string) (string, error) {
	if c == nil || c.FS == nil {
		return "", errors.New("文件系统尚未初始化")
	}
	return c.FS.SelectDirectory(defaultDirectory)
}

// LoadConfigs 供前端调用：读取配置文件并返回解析后的结果。
func (c *Core) LoadConfigs() (*Configs, error) {
	if c == nil || c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}

	if err := LoadConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
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
	selected, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousConfig := c.Configs.EnablesConfig
	c.Configs.EnablesConfig = selected

	if err := SaveConfig(); err != nil {
		c.Configs.EnablesConfig = previousConfig
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}

// SelectModel 供前端调用：在当前启用的配置里选定要使用的模型，
// 记录到该配置的 EnabledModel 字段并写回配置文件。
func (c *Core) SelectModel(model string) (*Configs, error) {
	if c == nil || c.Paths == nil {
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

	if err := SaveConfig(); err != nil {
		c.Configs.EnablesConfig.EnabledModel = previousModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
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
	if c == nil || c.Paths == nil {
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

	entry := Config{
		ID:            newID(),
		Name:          strings.TrimSpace(name),
		ServerAddress: serverAddress,
		Protocol:      protocol,
		APIKey:        apiKey,
		Model:         cleanModels,
		EnabledModel:  cleanModels[0].ID,
	}
	if c.Configs.ALLConfig == nil {
		all := []Config{}
		c.Configs.ALLConfig = &all
	}
	previousEnabled := c.Configs.EnablesConfig
	*c.Configs.ALLConfig = append(*c.Configs.ALLConfig, entry)
	c.Configs.EnablesConfig = &(*c.Configs.ALLConfig)[len(*c.Configs.ALLConfig)-1]
	if err := SaveConfig(); err != nil {
		*c.Configs.ALLConfig = (*c.Configs.ALLConfig)[:len(*c.Configs.ALLConfig)-1]
		c.Configs.EnablesConfig = previousEnabled
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}
	return c.Configs, nil
}

// RefreshModelConfig 更新已有配置探测到的聊天地址和模型列表。
func (c *Core) RefreshModelConfig(index int, serverAddress string, models []apiImpl.Model) (*Configs, error) {
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
	if err := SaveConfig(); err != nil {
		entry.ServerAddress = previousAddress
		entry.Model = previousModels
		entry.EnabledModel = previousEnabledModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}
	return c.Configs, nil
}

// RenameConfig 供前端调用：修改第 index 个配置的名称并写回配置文件。
// 名称首尾空白会被去掉；留空表示不命名，前端展示时回退为服务地址。
func (c *Core) RenameConfig(index int, name string) (*Configs, error) {
	entry, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousName := entry.Name
	entry.Name = strings.TrimSpace(name)

	if err := SaveConfig(); err != nil {
		entry.Name = previousName
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}

// DeleteConfig 供前端调用：删除第 index 个配置并写回配置文件。
// 若删除的是当前启用的配置，启用状态一并清空。
func (c *Core) DeleteConfig(index int) (*Configs, error) {
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

	previousAll := slices.Clone(all)
	previousEnables := c.Configs.EnablesConfig

	removed := &all[index]
	*c.Configs.ALLConfig = append(all[:index:index], all[index+1:]...)
	if c.Configs.EnablesConfig == removed {
		c.Configs.EnablesConfig = nil
	}

	if err := SaveConfig(); err != nil {
		*c.Configs.ALLConfig = previousAll
		c.Configs.EnablesConfig = previousEnables
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
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
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (c *Core) readChatIndexLocked(directory string) (chatIndex, error) {
	data, err := c.FS.ReadFile(filepath.Join(directory, "index.json"))
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
			data, err := c.FS.ReadFile(filepath.Join(directory, id+".json"))
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
	data, err := c.FS.ReadFile(filepath.Join(directory, id+".json"))
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

func (c *Core) configByID(id string) (*Config, error) {
	if c == nil || c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("模型配置尚未加载")
	}
	for i := range *c.Configs.ALLConfig {
		if (*c.Configs.ALLConfig)[i].ID == id {
			return &(*c.Configs.ALLConfig)[i], nil
		}
	}
	return nil, fmt.Errorf("会话引用的配置 %q 不存在，请重新选择配置", id)
}

func (c *Core) ListChats() ([]chat.ChatSummary, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c == nil || c.Configs == nil {
		return nil, errors.New("模型配置尚未加载")
	}
	if configID == "" {
		if c.Configs.EnablesConfig == nil {
			return nil, errors.New("尚未选择配置（提供商），请先选择配置")
		}
		configID = c.Configs.EnablesConfig.ID
	}
	cfg, err := c.configByID(configID)
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
	session := chat.NewChatTask(newID(), title, configID, model, tree)
	session.CreatedAt = now
	session.UpdatedAt = now
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	if err := c.saveChatLocked(directory, session); err != nil {
		_ = c.FS.DeleteFile(filepath.Join(directory, session.ID+".json"))
		return nil, err
	}
	return session, nil
}

func (c *Core) GetChat(id string) (*chat.ChatTask, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	return c.readChatLocked(directory, id)
}

func (c *Core) RenameChat(id, title string) (*chat.ChatTask, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
		return errors.New("对话正在生成中，暂时无法删除")
	}
	delete(c.ChatList, id)
	directory, err := c.chatDirectory()
	if err != nil {
		return err
	}
	if _, err := c.readChatLocked(directory, id); err != nil {
		return err
	}
	if err := c.FS.DeleteFile(filepath.Join(directory, id+".json")); err != nil {
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
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
	delete(c.ChatList, id)
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Core) SetChatBranch(id, parentID string, choice int) (*chat.ChatTask, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
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
	if err := session.Tree.SelectBranch(parentID, choice); err != nil {
		return nil, err
	}
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
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

type ToolKitDescriptor struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	DefaultEnabled bool     `json:"defaultEnabled"`
	Tools          []string `json:"tools"`
}

type ToolSetDescriptor struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	DefaultEnabled bool                `json:"defaultEnabled"`
	Kits           []ToolKitDescriptor `json:"kits"`
}

// ListToolSets 返回启动期注册的所有工具集，供前端渲染开关。
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
			names := make([]string, 0, len(kitMeta.Tools))
			for _, tool := range kit.List() {
				if tm := tool.Meta(); tm != nil {
					names = append(names, tm.Name)
				}
			}
			sort.Strings(names)
			item.Kits = append(item.Kits, ToolKitDescriptor{
				ID: kitID, Name: kitMeta.Name, Description: kitMeta.Description,
				DefaultEnabled: meta.Enabled && kitMeta.Enabled,
				Tools:          names,
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
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
	if cfg.Sets == nil && cfg.Permission == 0 {
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
	var enabled []apiImpl.Tool
	for setID, set := range agenttool.ToolSets {
		meta := set.Meta()
		for kitID, kit := range meta.ToolKits {
			on, explicit := session.ToolConfig.Kit(setID, kitID)
			if !explicit && c.Configs != nil {
				on, explicit = c.Configs.ToolNormalConfig.Kit(setID, kitID)
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
				if converted, ok := agenttool.ToolList[tm.ID]; ok {
					enabled = append(enabled, converted)
				}
			}
		}
	}
	return enabled
}

func (c *Core) SendChatMessage(id, text, reasoningEffort string) error {
	text = strings.TrimSpace(text)
	if text == "" {
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
	userNode := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "user", Content: text})
	if len(path) == 0 {
		session.Tree.Root.Add(userNode)
	} else if err := session.Tree.AddNext(path[len(path)-1].ID, userNode); err != nil {
		return err
	}
	if session.Title == "新建对话" {
		session.Title = titleFromMessage(text)
	}
	requestMessages, err := session.Tree.MessagesThrough(userNode.ID)
	if err != nil {
		return err
	}
	replyNode := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(userNode.ID, replyNode); err != nil {
		return err
	}
	return c.runChatNode(id, session, config, directory, replyNode, requestMessages, reasoningEffort)
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
	requestMessages, err := session.Tree.MessagesThrough(parent.ID)
	if err != nil {
		return err
	}
	newReply := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(parent.ID, newReply); err != nil {
		return err
	}
	return c.runChatNode(id, session, config, directory, newReply, requestMessages, reasoningEffort)
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
	userNode := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "user", Content: text})
	if err := session.Tree.AddNext(parentID, userNode); err != nil {
		return err
	}
	requestMessages, err := session.Tree.MessagesThrough(userNode.ID)
	if err != nil {
		return err
	}
	replyNode := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(userNode.ID, replyNode); err != nil {
		return err
	}
	return c.runChatNode(id, session, config, directory, replyNode, requestMessages, reasoningEffort)
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
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending == nil {
		c.chatSending = make(map[string]bool)
	}
	if c.chatSending[id] {
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
	configSnapshot := *config
	c.chatSending[id] = true
	return session, &configSnapshot, directory, nil
}

func (c *Core) endChatSend(id string) {
	c.chatMu.Lock()
	delete(c.chatSending, id)
	c.chatMu.Unlock()
}

// 一轮工具调用后模型会带着结果重新请求，限制轮数防止模型无限调用。
const maxToolRounds = 10

func (c *Core) runChatNode(id string, session *chat.ChatTask, config *Config, directory string, reply *chatstruct.ChatNode, messages []apiImpl.ChatMessage, reasoningEffort string) error {
	toolContext, err := c.toolContextFor(session)
	if err != nil {
		return err
	}
	// 每次生成都把本次使用的思考强度写进会话，随后 saveChatLocked 会一并落盘。
	session.ReasoningEffort = normalizeReasoningEffort(reasoningEffort)
	reply.ReasoningEffort = session.ReasoningEffort
	session.UpdatedAt = time.Now().UTC()
	c.chatMu.Lock()
	err = c.saveChatLocked(directory, session)
	c.chatMu.Unlock()
	if err != nil {
		return err
	}

	c.chatMu.Lock()
	if c.ChatList == nil {
		c.ChatList = make(map[string]chat.ChatTask)
	}
	entry := c.ChatList[id]
	if entry.Client == nil || entry.ConfigID != session.ConfigID || entry.Model != session.Model {
		entry = chat.ChatTask{
			ConfigID: session.ConfigID,
			Model:    session.Model,
			Client: netclient.NewAPIClient(netclient.Options{
				ServerAddress: config.ServerAddress,
				APIKey:        config.APIKey,
				Model:         session.Model,
			}),
		}
		c.ChatList[id] = entry
	}
	session.Client = entry.Client
	c.chatMu.Unlock()

	tools := c.enabledTools(session)
	// history 是本次请求的历史，produced 是本节点新产出的消息（assistant / tool 交替）
	// 人设作为第一条 system，每次请求都带上——模型服务端不保存上一轮状态
	history := make([]apiImpl.ChatMessage, 0, len(messages)+1)
	if session.Tree != nil && session.Tree.Root != nil {
		active := persona.Resolve(session.Tree.Root.PersonaID)
		if prompt, promptErr := active.Prompt(); promptErr == nil {
			history = append(history, apiImpl.ChatMessage{Role: "system", Content: prompt})
		}
	}
	if toolContext.WorkSpace != "" {
		history = append(history, apiImpl.ChatMessage{Role: "system", Content: "当前工作区路径：" + toolContext.WorkSpace + "。文件工具的相对路径以此目录为起点。"})
	} else {
		history = append(history, apiImpl.ChatMessage{Role: "system", Content: "当前未使用工作区。自动编辑权限下只允许读取文件，不允许创建或修改文件。"})
	}
	history = append(history, messages...)
	produced := make([]apiImpl.ChatMessage, 0, 4)
	reply.Child = produced

	for round := 0; round <= maxToolRounds; round++ {
		assistant := apiImpl.ChatMessage{Role: "assistant"}
		request := apiImpl.ChatRequest{
			Model:    session.Model,
			Messages: history,
			Thinking: session.ReasoningEffort,
			Tools:    tools,
		}
		streamErr := session.Client.StreamChat(context.Background(), request, &apiImpl.ChatCompletionAPI{}, func(delta apiImpl.Delta) {
			if delta.Content != "" {
				assistant.Content += delta.Content
			}
			if delta.Reasoning != "" {
				assistant.Reasoning += delta.Reasoning
			}
			if len(delta.ToolCalls) > 0 {
				for i := range delta.ToolCalls {
					delta.ToolCalls[i].DisplayName = agenttool.DisplayName(delta.ToolCalls[i].Name)
				}
				assistant.ToolCalls = append(assistant.ToolCalls, delta.ToolCalls...)
			}
			if delta.Usage != nil {
				usage := *delta.Usage
				reply.Usage = &usage
			}
			if delta.Content != "" || delta.Reasoning != "" || delta.Usage != nil {
				EmitJSON(EventChatDelta, map[string]any{
					"chatId": id, "nodeId": reply.ID, "content": delta.Content,
					"reasoning": delta.Reasoning, "usage": delta.Usage,
				})
			}
		})
		produced = append(produced, assistant)
		reply.Child = produced
		if streamErr != nil {
			c.saveReplyLocked(directory, session)
			return streamErr
		}

		// 没有工具调用说明这一轮就是最终回复
		if len(assistant.ToolCalls) == 0 {
			break
		}

		history = append(history, assistant)
		for _, call := range assistant.ToolCalls {
			EmitJSON(EventChatTool, map[string]string{
				"chatId": id, "nodeId": reply.ID, "toolId": call.Name, "callId": call.ID,
				"name": call.DisplayName, "event": "start",
			})
			output, callErr := executeToolCall(call, toolContext)
			if callErr != nil {
				EmitJSON(EventChatTool, map[string]string{
					"chatId": id, "nodeId": reply.ID, "toolId": call.Name, "callId": call.ID,
					"name":  call.DisplayName,
					"event": "fail", "error": callErr.Error(),
				})
				// 工具失败不能中断整轮，回一条 tool 消息让模型自己处理
				output = "工具执行失败：" + callErr.Error()
			} else {
				EmitJSON(EventChatTool, map[string]string{
					"chatId": id, "nodeId": reply.ID, "toolId": call.Name, "callId": call.ID,
					"name": call.DisplayName, "event": "done",
				})
			}
			toolMsg := apiImpl.ChatMessage{Role: "tool", ToolCallID: call.ID, Content: output}
			history = append(history, toolMsg)
			produced = append(produced, toolMsg)
			reply.Child = produced
		}

		session.UpdatedAt = time.Now().UTC()
		if err := c.saveReplyLocked(directory, session); err != nil {
			return err
		}
	}

	return c.saveReplyLocked(directory, session)
}

// executeToolCall 按 API 函数名（ToolMeta.ID）取工具实例、解析参数并执行，带超时保护。
func executeToolCall(call apiImpl.ToolCall, ctx *agenttool.ToolContext) (string, error) {
	tool := agenttool.Tools[call.Name]
	if tool == nil {
		return "", fmt.Errorf("未找到工具 %q", call.Name)
	}
	meta := tool.Meta()
	if meta == nil {
		return "", errors.New("工具缺少元信息")
	}
	required := meta.Permission
	if required == 0 {
		required = agenttool.PermissionReadOnly
	}
	if ctx == nil || !ctx.Check(required) {
		if ctx != nil && ctx.WorkSpace == "" && ctx.PermissionLevel == agenttool.PermissionAutoEdit && required >= agenttool.PermissionAutoEdit {
			return "", errors.New("未使用工作区，自动编辑权限下禁止写入文件")
		}
		return "", fmt.Errorf("没有执行工具 %q 所需的权限", meta.Name)
	}
	args := map[string]any{}
	if raw := strings.TrimSpace(call.Arguments); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return "", fmt.Errorf("参数解析失败：%w", err)
		}
	}
	timeout, err := meta.TimeoutFor(args)
	if err != nil {
		return "", err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := tool.Fun(ctx, args)
		done <- result{output: output, err: err}
	}()

	select {
	case r := <-done:
		return r.output, r.err
	case <-timer.C:
		return "", fmt.Errorf("执行超时（%s）", timeout)
	}
}

func (c *Core) saveReplyLocked(directory string, session *chat.ChatTask) error {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	return c.saveChatLocked(directory, session)
}
