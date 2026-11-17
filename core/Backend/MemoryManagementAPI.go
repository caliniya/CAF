package Backend

import (
	"context"
	"errors"
	"strings"

	memory "github.com/caliniya/caf/Memory"
)

// 调用方持有对话锁。已有对话的工作区由磁盘记录确定，不信任前端传入的工作区。
func (c *Core) memoryWorkspaceLocked(chatID, workspaceID string) (string, error) {
	if chatID != "" {
		server, err := c.Servers.Get[*ChatServer](ChatServerName)
		if err != nil {
			return "", err
		}
		if server.sending[chatID] != nil {
			return "", errors.New("对话正在生成中，暂时无法管理记忆")
		}
		directory, err := c.chatDirectory()
		if err != nil {
			return "", err
		}
		session, err := c.readChatLocked(directory, chatID)
		if err != nil {
			return "", err
		}
		workspaceID = session.WorkspaceID
	}
	if _, err := c.workspaceByIDLocked(workspaceID); err != nil {
		return "", err
	}
	return workspaceID, nil
}

func (c *Core) memoryTargetLocked(chatID, workspaceID, libraryID string) (*memory.MemoryServer, *memory.RecollectionMeta, string, error) {
	workspaceID, err := c.memoryWorkspaceLocked(chatID, workspaceID)
	if err != nil {
		return nil, nil, "", err
	}
	server, err := c.memoryServer(context.Background())
	if err != nil {
		return nil, nil, "", err
	}
	meta, err := server.GetMeta(context.Background(), libraryID)
	if err != nil {
		return nil, nil, "", err
	}
	if meta.Scope == memory.ScopeWorkspace && (workspaceID == "" || meta.WorkspaceID != workspaceID) {
		return nil, nil, "", errors.New("记忆不属于当前工作区")
	}
	return server, meta, workspaceID, nil
}

func (c *Core) CreateMemoryLibrary(chatID, workspaceID, name, scope string) (*memory.RecollectionMeta, error) {
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	workspaceID, err = c.memoryWorkspaceLocked(chatID, workspaceID)
	if err != nil {
		return nil, err
	}
	if scope == memory.ScopeGlobal {
		workspaceID = ""
	}
	if scope == memory.ScopeWorkspace && workspaceID == "" {
		return nil, errors.New("当前对话未使用工作区")
	}
	server, err := c.memoryServer(context.Background())
	if err != nil {
		return nil, err
	}
	return server.CreateRecollection(context.Background(), strings.TrimSpace(name), "", scope, workspaceID)
}

func (c *Core) RenameMemoryLibrary(chatID, workspaceID, libraryID, name string) error {
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, meta, _, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return err
	}
	if meta.Name == memory.MainName {
		return errors.New("全局主记忆不能重命名")
	}
	return server.UpdateRecollection(context.Background(), libraryID, strings.TrimSpace(name), meta.Description)
}

func (c *Core) DeleteMemoryLibrary(chatID, workspaceID, libraryID string) error {
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, _, _, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return err
	}
	return server.DeleteRecollection(context.Background(), libraryID)
}

func (c *Core) SetMemoryLibraryScope(chatID, workspaceID, libraryID, scope string) error {
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, _, workspaceID, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return err
	}
	if scope == memory.ScopeGlobal {
		workspaceID = ""
	}
	if scope == memory.ScopeWorkspace && workspaceID == "" {
		return errors.New("当前对话未使用工作区")
	}
	return server.ChangeScope(context.Background(), libraryID, scope, workspaceID)
}
