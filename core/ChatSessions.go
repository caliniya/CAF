package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	netclient "github.com/caliniya/caf/NetClient"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

type ChatSession struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	ConfigID  string               `json:"configId"`
	Model     string               `json:"model"`
	Tree      *chatstruct.ChatTree `json:"tree"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type ChatSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ConfigID  string    `json:"configId"`
	Model     string    `json:"model"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type chatIndex struct {
	Chats []ChatSummary `json:"chats"`
}

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
		return chatIndex{Chats: []ChatSummary{}}, nil
	}
	if err != nil {
		return chatIndex{}, err
	}
	var index chatIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return chatIndex{}, fmt.Errorf("解析会话索引失败：%w", err)
	}
	if index.Chats == nil {
		index.Chats = []ChatSummary{}
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

func (c *Core) readChatLocked(directory, id string) (*ChatSession, error) {
	if !validChatID(id) {
		return nil, errors.New("无效会话 ID")
	}
	data, err := c.FS.ReadFile(filepath.Join(directory, id+".json"))
	if err != nil {
		return nil, err
	}
	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("解析会话失败：%w", err)
	}
	if session.ID != id {
		return nil, errors.New("会话文件 ID 不匹配")
	}
	if session.Tree == nil {
		session.Tree = chatstruct.CreateTree(session.Title)
	}
	return &session, nil
}

func (c *Core) saveChatLocked(directory string, session *ChatSession) error {
	if err := c.writeJSONLocked(filepath.Join(directory, session.ID+".json"), session); err != nil {
		return fmt.Errorf("保存会话失败：%w", err)
	}
	index, err := c.readChatIndexLocked(directory)
	if err != nil {
		return fmt.Errorf("读取会话索引失败：%w", err)
	}
	summary := ChatSummary{
		ID: session.ID, Title: session.Title, ConfigID: session.ConfigID,
		Model: session.Model, UpdatedAt: session.UpdatedAt,
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

func (c *Core) ListChats() ([]ChatSummary, error) {
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

func (c *Core) CreateChat(configID, model, title string) (*ChatSession, error) {
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
	if !slicesContainsModel(cfg, model) {
		return nil, fmt.Errorf("模型 %q 不属于所选配置", model)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新建对话"
	}
	now := time.Now().UTC()
	session := &ChatSession{
		ID: newID(), Title: title, ConfigID: configID, Model: model,
		Tree: chatstruct.CreateTree(title), CreatedAt: now, UpdatedAt: now,
	}
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

func (c *Core) GetChat(id string) (*ChatSession, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	return c.readChatLocked(directory, id)
}

func (c *Core) RenameChat(id, title string) (*ChatSession, error) {
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

func (c *Core) UpdateChatModel(id, configID, model string) (*ChatSession, error) {
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
	if !slicesContainsModel(cfg, model) {
		return nil, fmt.Errorf("模型 %q 不属于所选配置", model)
	}
	session.ConfigID = configID
	session.Model = model
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Core) SetChatBranch(id, parentID string, choice int) (*ChatSession, error) {
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

func slicesContainsModel(cfg *Config, model string) bool {
	if cfg == nil || model == "" {
		return false
	}
	for _, item := range cfg.Model {
		if item.ID == model {
			return true
		}
	}
	return false
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

func (c *Core) beginChatSend(id string) (*ChatSession, *Config, string, error) {
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
	if !slicesContainsModel(config, session.Model) {
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

func (c *Core) runChatNode(id string, session *ChatSession, config *Config, directory string, reply *chatstruct.ChatNode, messages []apiImpl.ChatMessage, reasoningEffort string) error {
	session.UpdatedAt = time.Now().UTC()
	c.chatMu.Lock()
	err := c.saveChatLocked(directory, session)
	c.chatMu.Unlock()
	if err != nil {
		return err
	}

	client := netclient.NewAPIClient(netclient.Options{
		ServerAddress: config.ServerAddress,
		APIKey:        config.APIKey,
		Model:         session.Model,
	})
	request := apiImpl.ChatRequest{Model: session.Model, Messages: messages, Thinking: reasoningEffort}
	generated := apiImpl.ChatMessage{Role: "assistant"}
	streamErr := client.StreamChat(context.Background(), request, &apiImpl.ChatCompletionAPI{}, func(delta apiImpl.Delta) {
		if delta.Content != "" {
			generated.Content += delta.Content
		}
		if delta.Reasoning != "" {
			generated.Reasoning += delta.Reasoning
		}
		if len(delta.ToolCalls) > 0 {
			generated.ToolCalls = append(generated.ToolCalls, delta.ToolCalls...)
		}
		if delta.Content != "" || delta.Reasoning != "" || delta.Usage != nil {
			EmitJSON(EventChatDelta, map[string]any{
				"chatId": id, "nodeId": reply.ID, "content": delta.Content,
				"reasoning": delta.Reasoning, "usage": delta.Usage,
			})
		}
		for _, tool := range delta.ToolCalls {
			EmitJSON(EventChatTool, map[string]string{"chatId": id, "nodeId": reply.ID, "name": tool.Name, "event": "start"})
			EmitJSON(EventChatTool, map[string]string{"chatId": id, "nodeId": reply.ID, "name": tool.Name, "event": "done"})
		}
	})
	reply.Child = []apiImpl.ChatMessage{generated}
	session.UpdatedAt = time.Now().UTC()
	c.chatMu.Lock()
	saveErr := c.saveChatLocked(directory, session)
	c.chatMu.Unlock()
	if streamErr != nil {
		return streamErr
	}
	return saveErr
}
