package Backend

import (
	"context"
	"errors"
	"strings"
	"time"

	memory "github.com/caliniya/caf/Memory"
)

// 详情仅组合现有元数据和当前正文，不改变记忆文件格式或读取历史。
type MemoryLibraryDetails struct {
	memory.RecollectionMeta
	Data map[string]memory.Memory `json:"data"`
}

type MemoryEntryInput struct {
	Name string   `json:"name"`
	Data string   `json:"data"`
	Date string   `json:"date,omitempty"`
	Tags []string `json:"tags"`
}

func validateMemoryEntryInput(input *MemoryEntryInput, editing bool) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return errors.New("记忆名称不能为空")
	}
	if strings.TrimSpace(input.Data) == "" {
		return errors.New("记忆内容不能为空")
	}
	if editing {
		if _, err := time.Parse(time.RFC3339Nano, input.Date); err != nil {
			return errors.New("记忆时间格式无效")
		}
	}
	return nil
}

func memoryLibraryDetails(server *memory.MemoryServer, libraryID string) (*MemoryLibraryDetails, error) {
	meta, err := server.GetMeta(context.Background(), libraryID)
	if err != nil {
		return nil, err
	}
	recollection, err := server.GetRecollection(context.Background(), libraryID)
	if err != nil {
		return nil, err
	}
	return &MemoryLibraryDetails{RecollectionMeta: *meta, Data: recollection.Data}, nil
}

func (c *Core) GetMemoryLibraryDetails(chatID, workspaceID, libraryID string) (*MemoryLibraryDetails, error) {
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, _, _, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return nil, err
	}
	return memoryLibraryDetails(server, libraryID)
}

func (c *Core) CreateMemoryEntry(chatID, workspaceID, libraryID string, input MemoryEntryInput) (*MemoryLibraryDetails, error) {
	if err := validateMemoryEntryInput(&input, false); err != nil {
		return nil, err
	}
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, _, _, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return nil, err
	}
	if _, err := server.AddNamedMemory(context.Background(), libraryID, input.Name, input.Data, input.Tags...); err != nil {
		return nil, err
	}
	return memoryLibraryDetails(server, libraryID)
}

func (c *Core) UpdateMemoryEntry(chatID, workspaceID, libraryID, memoryID string, input MemoryEntryInput) (*MemoryLibraryDetails, error) {
	if err := validateMemoryEntryInput(&input, true); err != nil {
		return nil, err
	}
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, _, _, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return nil, err
	}
	next := memory.CreateMemory(input.Data)
	next.Name, next.MemoryDate = input.Name, input.Date
	*next.Tag = append(*next.Tag, input.Tags...)
	if err := server.EditMemory(context.Background(), libraryID, memoryID, *next); err != nil {
		return nil, err
	}
	return memoryLibraryDetails(server, libraryID)
}

func (c *Core) DeleteMemoryEntry(chatID, workspaceID, libraryID, memoryID string) (*MemoryLibraryDetails, error) {
	chatServer, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	chatServer.mu.Lock()
	defer chatServer.mu.Unlock()
	server, _, _, err := c.memoryTargetLocked(chatID, workspaceID, libraryID)
	if err != nil {
		return nil, err
	}
	if err := server.DeleteMemory(context.Background(), libraryID, memoryID); err != nil {
		return nil, err
	}
	return memoryLibraryDetails(server, libraryID)
}

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
