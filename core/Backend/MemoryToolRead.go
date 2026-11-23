package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	memory "github.com/caliniya/caf/Memory"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
)

func (c *Core) readChatMemory(ctx context.Context, enabled []string, libraryID, memoryID string) (*memory.Memory, error) {
	if !slices.Contains(enabled, libraryID) {
		return nil, errors.New("该记忆库未在当前对话中启用")
	}
	server, err := c.memoryServer(ctx)
	if err != nil {
		return nil, err
	}
	return server.GetMemory(ctx, libraryID, memoryID)
}

func (c *Core) listChatMemories(ctx context.Context, enabled []string, libraryID string) (*memory.Recollection, error) {
	if !slices.Contains(enabled, libraryID) {
		return nil, errors.New("该记忆库未在当前对话中启用")
	}
	server, err := c.memoryServer(ctx)
	if err != nil {
		return nil, err
	}
	value, err := server.GetRecollection(ctx, libraryID)
	if err == nil && value.Name == memory.MainName {
		value.Name = memory.MainDisplayName
	}
	return value, err
}

func (c *Core) searchChatMemories(ctx context.Context, enabled []string, libraryID string, queries []string) (map[string]map[string]memory.Memory, error) {
	ids := slices.Clone(enabled)
	if libraryID != "" {
		if !slices.Contains(ids, libraryID) {
			return nil, errors.New("该记忆库未在当前对话中启用")
		}
		ids = []string{libraryID}
	}
	server, err := c.memoryServer(ctx)
	if err != nil {
		return nil, err
	}
	return server.Search(ctx, ids, queries...)
}

// 查询仅检查已启用库，不沿用写入时的工作区作用域限制。
func (c *Core) bindChatMemoryRead(ctx *agenttool.ToolContext, session *chat.ChatTask) {
	ids := slices.Clone(session.MemoryLibraryIDs)
	ctx.ReadMemory = func(parent context.Context, libraryID, memoryID string) (string, error) {
		value, err := c.readChatMemory(parent, ids, libraryID, memoryID)
		if err != nil {
			return "", err
		}
		return memoryReadJSON(libraryID, memoryID, value)
	}
	ctx.SearchMemory = func(parent context.Context, libraryID string, queries []string) (string, error) {
		value, err := c.searchChatMemories(parent, ids, libraryID, queries)
		if err != nil {
			return "", err
		}
		return memoryToolJSON(value)
	}
	ctx.ListMemories = func(parent context.Context, libraryID string) (string, error) {
		value, err := c.listChatMemories(parent, ids, libraryID)
		if err != nil {
			return "", err
		}
		return memoryToolJSON(value)
	}
}

func memoryToolJSON(value any) (string, error) {
	body, err := json.Marshal(value)
	return string(body), err
}

func memoryReadJSON(libraryID, memoryID string, value *memory.Memory) (string, error) {
	return memoryToolJSON(struct {
		Library string         `json:"recollection_id"`
		ID      string         `json:"id"`
		Memory  *memory.Memory `json:"memory"`
	}{libraryID, memoryID, value})
}
