package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	chat "github.com/caliniya/caf/core/Chat"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	persona "github.com/caliniya/caf/core/Persona"
	identity "github.com/caliniya/caf/utils/id"
)

type QQContextInfo struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`
	Kind      string         `json:"kind"`
	BotID     string         `json:"botId"`
	UserID    string         `json:"userId,omitempty"`
	GroupID   string         `json:"groupId,omitempty"`
	Workspace chat.WorkSpace `json:"workspace"`
	UpdatedAt time.Time      `json:"updatedAt"`
	// 最近一次对话请求的输入 + 输出用量；nil 表示模型未提供统计。
	ContextTokens      *int64 `json:"contextTokens"`
	ContextWarningSent bool   `json:"contextWarningSent"`
}

func (c *Core) qqChatDirectory(clientID, adapterID string) (string, error) {
	if err := validateExtensionClientID(clientID); err != nil {
		return "", err
	}
	if !identity.Valid(adapterID, identity.QQAdapter) {
		return "", errors.New("无效扩展 ID")
	}
	if c.Paths == nil || c.FS == nil || !filepath.IsAbs(c.Paths.Data) {
		return "", errors.New("QQ 对话存储尚未初始化")
	}
	return filepath.Join(c.Paths.Data, "ext", "clients", clientID, "qq", adapterID), nil
}

func (c *Core) readQQContexts(directory string) ([]QQContextInfo, error) {
	data, err := c.readConversationFile(filepath.Join(directory, "contexts.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return []QQContextInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result []QQContextInfo
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析 QQ 上下文索引：%w", err)
	}
	seen, keys := map[string]bool{}, map[string]bool{}
	for _, item := range result {
		if !validChatID(item.ID) || !identity.Valid(item.Workspace.ID, identity.Workspace) || item.Key == "" || seen[item.ID] || keys[item.Key] {
			return nil, errors.New("QQ 上下文索引中的 ID 或路由无效")
		}
		seen[item.ID], keys[item.Key] = true, true
	}
	return result, nil
}

func (c *Core) qqWorkspacePath(adapterID, workspaceID string) (string, error) {
	if c.Paths == nil || !filepath.IsAbs(c.Paths.InternalWorkspaces) {
		return "", errors.New("内部工作区路径尚未初始化")
	}
	if !identity.Valid(adapterID, identity.QQAdapter) || !identity.Valid(workspaceID, identity.Workspace) {
		return "", errors.New("内部工作区 ID 无效")
	}
	return filepath.Join(c.Paths.InternalWorkspaces, adapterID, workspaceID), nil
}

func (r *qqChatRuntime) loadContextsLocked() error {
	items, err := r.core.readQQContexts(r.directory)
	if err != nil {
		return err
	}
	for _, item := range items {
		path, err := r.core.qqWorkspacePath(r.adapterID, item.Workspace.ID)
		if err != nil {
			return err
		}
		// 工作区位置只由宿主计算，索引中的路径不能改变工具的目录。
		item.Workspace.Path = path
		r.records[item.Key] = item
	}
	return nil
}

func (r *qqChatRuntime) saveContextsLocked() error {
	r.storageMu.Lock()
	defer r.storageMu.Unlock()
	if r.storageClosed {
		return context.Canceled
	}
	items := make([]QQContextInfo, 0, len(r.records))
	for _, item := range r.records {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return r.core.writeJSONLocked(filepath.Join(r.directory, "contexts.json"), items)
}

func (r *qqChatRuntime) ensureContextLocked(job qqChatJob) (QQContextInfo, error) {
	if record, exists := r.records[job.key]; exists {
		return record, nil
	}
	chatID, err := identity.New(identity.Chat)
	if err != nil {
		return QQContextInfo{}, err
	}
	workspaceID, err := identity.New(identity.Workspace)
	if err != nil {
		return QQContextInfo{}, err
	}
	path, err := r.core.qqWorkspacePath(r.adapterID, workspaceID)
	if err != nil {
		return QQContextInfo{}, err
	}
	if err := r.core.FS.CreateDirectory(path, 0o700); err != nil {
		return QQContextInfo{}, err
	}
	if err := r.core.FS.CreateDirectory(filepath.Join(r.directory, "sessions"), 0o700); err != nil {
		return QQContextInfo{}, err
	}
	name := "QQ " + job.message.UserID
	if job.kind == "group" {
		name = "QQ 群 " + job.message.GroupID
	}
	if job.kind == "agent" {
		name += " agent"
	}
	userID, groupID := job.message.UserID, ""
	if job.kind == "group" {
		userID, groupID = "", job.message.GroupID
	}
	record := QQContextInfo{ID: chatID, Key: job.key, Kind: job.kind, BotID: job.message.SelfID, UserID: userID, GroupID: groupID,
		Workspace: chat.WorkSpace{ID: workspaceID, Name: name, Path: path}, UpdatedAt: time.Now().UTC(), ContextTokens: new(int64)}
	session := chat.NewChatTask(chatID, name, job.config.ConfigID, job.config.Model, nil)
	session.WorkspaceID = workspaceID
	session.Tree.Root.PersonaID = job.config.PersonaID
	if job.kind == "agent" {
		session.Tree.Root.PersonaID = persona.DefaultID
	}
	if err := r.saveSession(session); err != nil {
		return QQContextInfo{}, err
	}
	r.records[job.key] = record
	if err := r.saveContextsLocked(); err != nil {
		delete(r.records, job.key)
		return QQContextInfo{}, err
	}
	return record, nil
}

func (r *qqChatRuntime) readSession(record QQContextInfo) (*chat.ChatTask, error) {
	data, err := r.core.readConversationFile(filepath.Join(r.directory, "sessions", record.ID+".json"))
	if err != nil {
		return nil, err
	}
	var session chat.ChatTask
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	if session.ID != record.ID || session.WorkspaceID != record.Workspace.ID {
		return nil, errors.New("QQ 上下文文件与索引不一致")
	}
	if session.Tree == nil || session.Tree.Root == nil {
		return nil, errors.New("QQ 上下文树无效")
	}
	fillToolDisplayNames(session.Tree)
	return &session, nil
}

func (r *qqChatRuntime) saveSession(session *chat.ChatTask) error {
	r.storageMu.Lock()
	defer r.storageMu.Unlock()
	if r.storageClosed {
		return context.Canceled
	}
	return r.core.writeJSONLocked(filepath.Join(r.directory, "sessions", session.ID+".json"), session)
}

func (c *Core) ListQQContexts(clientID, extensionID string) ([]QQContextInfo, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	if _, _, err := c.clientExtensionLocked(clientID, extensionID); err != nil {
		return nil, err
	}
	q, err := c.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		return nil, err
	}
	q.chatMu.Lock()
	r := q.chats[extensionID]
	q.chatMu.Unlock()
	if r != nil && r.clientID == clientID {
		r.mu.Lock()
		defer r.mu.Unlock()
		items := make([]QQContextInfo, 0, len(r.records))
		for _, item := range r.records {
			items = append(items, item)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
		return items, nil
	}
	directory, err := c.qqChatDirectory(clientID, extensionID)
	if err != nil {
		return nil, err
	}
	return c.readQQContexts(directory)
}

// 清除只重置对话树；旧树归档，模型配置、工作区文件和长期记忆保留。
func (c *Core) ClearQQContext(clientID, extensionID, contextID string) error {
	return c.resetQQContext(clientID, extensionID, contextID, false)
}

// 删除取消该上下文的任务，移除对话、归档和内部工作区；共享个人记忆保留。
func (c *Core) DeleteQQContext(clientID, extensionID, contextID string) error {
	return c.resetQQContext(clientID, extensionID, contextID, true)
}

func (c *Core) resetQQContext(clientID, extensionID, contextID string, remove bool) error {
	action := "清除"
	if remove {
		action = "删除"
	}
	c.extMu.Lock()
	if _, _, err := c.clientExtensionLocked(clientID, extensionID); err != nil {
		c.extMu.Unlock()
		return err
	}
	q, err := c.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		c.extMu.Unlock()
		return err
	}
	q.chatMu.Lock()
	r := q.chats[extensionID]
	q.chatMu.Unlock()
	offline := r == nil
	if r == nil {
		directory, pathErr := c.qqChatDirectory(clientID, extensionID)
		if pathErr != nil {
			c.extMu.Unlock()
			return pathErr
		}
		r = &qqChatRuntime{core: c, server: q, clientID: clientID, adapterID: extensionID, directory: directory,
			records: map[string]QQContextInfo{}, workers: map[string]*qqContextWorker{}, clearing: map[string]bool{}}
		r.epochs = map[string]uint64{}
		r.clearedAt = map[string]time.Time{}
		if err := r.loadContextsLocked(); err != nil {
			c.extMu.Unlock()
			return err
		}
	}
	// 先标记清除并取消任务，再释放配置锁，等待生成结束时不会阻塞其他消息路由。
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		c.extMu.Unlock()
		select {
		case <-r.done:
			return c.resetQQContext(clientID, extensionID, contextID, remove)
		case <-time.After(15 * time.Second):
			return fmt.Errorf("QQ 上下文正在关闭，请稍后重试%s", action)
		}
	}
	keys := []string{}
	for key, record := range r.records {
		if contextID == "" || record.ID == contextID {
			keys = append(keys, key)
		}
	}
	if contextID != "" && len(keys) == 0 {
		r.mu.Unlock()
		c.extMu.Unlock()
		return errors.New("QQ 上下文不存在")
	}
	for _, key := range keys {
		if r.clearing[key] {
			r.mu.Unlock()
			c.extMu.Unlock()
			return fmt.Errorf("QQ 上下文正在%s", action)
		}
	}
	if !offline {
		r.wg.Add(1)
		defer r.wg.Done()
	}
	waits := []<-chan struct{}{}
	for _, key := range keys {
		r.clearing[key] = true
		r.epochs[key]++
		r.clearedAt[key] = time.Now()
		if worker := r.workers[key]; worker != nil {
			worker.cancel()
			waits = append(waits, worker.done)
		}
	}
	r.cancelDecisionsLocked()
	r.recent, r.batches = map[string][]qqRecentMessage{}, map[string][]qqChatJob{}
	r.senderRecent = map[string][]qqRecentMessage{}
	r.mu.Unlock()
	if offline {
		defer c.extMu.Unlock()
	} else {
		c.extMu.Unlock()
	}
	defer func() {
		r.mu.Lock()
		for _, key := range keys {
			delete(r.clearing, key)
		}
		r.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, done := range waits {
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("旧 QQ 生成尚未结束，请稍后重试%s", action)
		}
	}
	if !offline {
		c.extMu.Lock()
		defer c.extMu.Unlock()
		if _, _, err := c.clientExtensionLocked(clientID, extensionID); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if remove {
		return r.deleteContextsLocked(keys)
	}
	for _, key := range keys {
		record := r.records[key]
		session, err := r.readSession(record)
		if err != nil {
			return err
		}
		archiveID, err := identity.New(identity.Archive)
		if err != nil {
			return err
		}
		archive := filepath.Join(r.directory, "history", session.ID)
		if err := c.FS.CreateDirectory(archive, 0o700); err != nil {
			return err
		}
		if err := c.writeJSONLocked(filepath.Join(archive, archiveID+".json"), session); err != nil {
			return err
		}
		personaID := session.Tree.Root.PersonaID
		session.Tree = chatstruct.CreateTree(session.Title)
		session.Tree.Root.PersonaID = personaID
		session.UpdatedAt = time.Now().UTC()
		if err := r.saveSession(session); err != nil {
			return err
		}
		record.UpdatedAt = session.UpdatedAt
		record.ContextTokens = new(int64)
		record.ContextWarningSent = false
		r.records[key] = record
	}
	return r.saveContextsLocked()
}

func (r *qqChatRuntime) deleteContextsLocked(keys []string) error {
	// 在递归删除前校验所有目标；删除路径只从宿主根目录和合法 ID 生成。
	for _, key := range keys {
		record := r.records[key]
		if !identity.Valid(record.ID, identity.Chat) {
			return errors.New("待删除的 QQ 上下文 ID 无效")
		}
		if _, err := r.core.qqWorkspacePath(r.adapterID, record.Workspace.ID); err != nil {
			return err
		}
	}
	err := func() error {
		r.storageMu.Lock()
		defer r.storageMu.Unlock()
		if r.storageClosed {
			return context.Canceled
		}
		deleted := map[string]bool{}
		for _, key := range keys {
			record := r.records[key]
			for _, relative := range []string{
				filepath.Join("sessions", record.ID+".json"), filepath.Join("sessions", record.ID+".json.tmp"), filepath.Join("history", record.ID), filepath.Join("attachments", record.ID),
			} {
				if err := r.core.FS.RemoveAllInRoot(r.directory, relative); err != nil {
					return fmt.Errorf("删除 QQ 对话文件失败：%w", err)
				}
			}
			if err := r.core.FS.RemoveAllInRoot(r.core.Paths.InternalWorkspaces, filepath.Join(r.adapterID, record.Workspace.ID)); err != nil {
				return fmt.Errorf("删除 QQ 内部工作区失败：%w", err)
			}
			if err := r.core.FS.RemoveAllInRoot(r.core.Paths.InternalWorkspaces, filepath.Join("WorkspaceCache", record.Workspace.ID)); err != nil {
				return fmt.Errorf("删除 QQ 工作区恢复缓存失败：%w", err)
			}
			deleted[key] = true
		}
		items := make([]QQContextInfo, 0, len(r.records)-len(keys))
		for key, item := range r.records {
			if !deleted[key] {
				items = append(items, item)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		return r.core.writeJSONLocked(filepath.Join(r.directory, "contexts.json"), items)
	}()
	if err != nil {
		return err
	}
	chatServer, _ := r.core.Servers.Get[*ChatServer](ChatServerName)
	// 释放写入锁后再获取 ChatServer.mu，避免与其他上下文的保存形成反向等待。
	if chatServer != nil {
		chatServer.mu.Lock()
		for _, key := range keys {
			delete(chatServer.ChatList, r.records[key].ID)
		}
		chatServer.mu.Unlock()
	}
	for _, key := range keys {
		delete(r.records, key)
	}
	return nil
}

func (r *qqChatRuntime) contextIDForKey(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.records[key].ID
}

func qqContextKey(botID, kind, targetID string) string {
	return strings.Join([]string{botID, kind, targetID}, ":")
}
