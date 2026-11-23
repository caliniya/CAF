package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	memory "github.com/caliniya/caf/Memory"
	netapi "github.com/caliniya/caf/NetClient/NetAPI"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	identity "github.com/caliniya/caf/utils/id"
	texts "github.com/caliniya/caf/utils/text"
)

type qqPersonalMemory struct {
	server    *memory.MemoryServer
	libraryID string
}

// 内部个人记忆使用独立存储，不加入桌面对话可选记忆列表。
// 同一机器人下，普通聊天和 agent 聊天共用该人的库。
func (q *QQServer) personalMemory(ctx context.Context, botID, userID string) (*qqPersonalMemory, error) {
	if _, err := parseQQNumbers(botID + " " + userID); err != nil || botID == "" || userID == "" {
		return nil, errors.New("无法识别个人记忆的 QQ 账号")
	}
	key := botID + ":" + userID
	q.memoryMu.Lock()
	defer q.memoryMu.Unlock()
	if value := q.memories[key]; value != nil {
		return value, nil
	}
	var clock *netapi.TimeClient
	if q.core.APIClient != nil {
		clock = q.core.APIClient.TimeClient
	}
	server := memory.NewMemoryServer(filepath.Join(q.core.Paths.Data, "qq", "memories", botID, userID), q.core.FS, clock)
	if err := server.Init(ctx); err != nil {
		return nil, err
	}
	items, err := server.ListRecollections(ctx)
	if err != nil {
		return nil, err
	}
	var value qqPersonalMemory
	value.server = server
	for _, item := range items {
		if item.Name == "个人记忆" {
			value.libraryID = item.ID
			break
		}
	}
	if value.libraryID == "" {
		workspaceID, err := identity.New(identity.Workspace)
		if err != nil {
			return nil, err
		}
		item, err := server.EnsureWorkspace(ctx, workspaceID, "个人记忆")
		if err != nil {
			return nil, err
		}
		value.libraryID = item.ID
	}
	if q.memories == nil {
		q.memories = make(map[string]*qqPersonalMemory)
	}
	q.memories[key] = &value
	return &value, nil
}

func (r *qqChatRuntime) bindPersonalMemory(ctx context.Context, session *chat.ChatTask, botID, userID string, options *chatExecutionOptions) error {
	value, err := r.server.personalMemory(ctx, botID, userID)
	if err != nil {
		return err
	}
	contents, err := value.server.GetRecollection(ctx, value.libraryID)
	if err != nil {
		return err
	}
	meta, err := value.server.GetMeta(ctx, value.libraryID)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		ID       string                   `json:"id"`
		Name     string                   `json:"name"`
		UserID   string                   `json:"userId"`
		Memories map[string]memory.Memory `json:"memories"`
	}{meta.ID, meta.Name, userID, contents.Data})
	if err != nil {
		return err
	}
	session.MemoryLibraryIDs = []string{value.libraryID}
	options.memoryContext = texts.QQPersonalMemory + string(body)
	options.memoryScope = func(callCtx context.Context, libraryID string) (bool, error) {
		if libraryID != value.libraryID {
			return false, errors.New("当前只能访问发送者的个人记忆")
		}
		_, err := value.server.GetMeta(callCtx, libraryID)
		return err == nil, err
	}
	options.createMemory = func(callCtx *agenttool.ToolContext, libraryID, data string, tags []string) (string, error) {
		if !callCtx.Check(agenttool.PermissionAutoEdit) || libraryID != value.libraryID {
			return "", errors.New("当前权限或个人记忆目标无效")
		}
		if strings.TrimSpace(data) == "" {
			return "", errors.New("记忆内容不能为空")
		}
		return value.server.AddMemory(callCtx.Context, libraryID, data, tags...)
	}
	options.readMemory = func(parent context.Context, libraryID, memoryID string) (string, error) {
		if libraryID != value.libraryID {
			return "", errors.New("当前只能访问发送者的个人记忆")
		}
		entry, err := value.server.GetMemory(parent, libraryID, memoryID)
		if err != nil {
			return "", err
		}
		return memoryReadJSON(libraryID, memoryID, entry)
	}
	options.searchMemory = func(parent context.Context, libraryID string, queries []string) (string, error) {
		if libraryID != "" && libraryID != value.libraryID {
			return "", errors.New("当前只能访问发送者的个人记忆")
		}
		entries, err := value.server.Search(parent, []string{value.libraryID}, queries...)
		if err != nil {
			return "", err
		}
		return memoryToolJSON(entries)
	}
	options.listMemories = func(parent context.Context, libraryID string) (string, error) {
		if libraryID != value.libraryID {
			return "", errors.New("当前只能访问发送者的个人记忆")
		}
		entries, err := value.server.GetRecollection(parent, libraryID)
		if err != nil {
			return "", err
		}
		return memoryToolJSON(entries)
	}
	return nil
}
