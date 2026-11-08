package Core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
)

type workspaceIndex struct {
	Workspaces []chat.WorkSpace `json:"workspaces"`
}

func (c *Core) workspaceFile() (string, error) {
	if c == nil || c.Paths == nil || c.FS == nil {
		return "", errors.New("工作区存储尚未初始化")
	}
	if err := c.FS.CreateDirectory(c.Paths.Data, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(c.Paths.Data, "workspaces.json"), nil
}

func (c *Core) readWorkspacesLocked() (workspaceIndex, error) {
	file, err := c.workspaceFile()
	if err != nil {
		return workspaceIndex{}, err
	}
	data, err := c.FS.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return workspaceIndex{Workspaces: []chat.WorkSpace{}}, nil
	}
	if err != nil {
		return workspaceIndex{}, err
	}
	var index workspaceIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return workspaceIndex{}, fmt.Errorf("解析工作区列表失败：%w", err)
	}
	if index.Workspaces == nil {
		index.Workspaces = []chat.WorkSpace{}
	}
	return index, nil
}

func (c *Core) ListWorkspaces() ([]chat.WorkSpace, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	index, err := c.readWorkspacesLocked()
	return index.Workspaces, err
}

func (c *Core) validateWorkspacePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("请选择工作区文件夹或填写绝对路径")
	}
	path = filepath.Clean(path)
	info, err := c.FS.Stat(path)
	if err != nil {
		return "", fmt.Errorf("无法访问工作区文件夹：%w", err)
	}
	if !info.IsDir() {
		return "", errors.New("工作区路径必须是文件夹")
	}
	return path, nil
}

func (c *Core) CreateWorkspace(name, path string) (*chat.WorkSpace, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	name = strings.TrimSpace(name)
	index, err := c.readWorkspacesLocked()
	if err != nil {
		return nil, err
	}
	path, err = c.validateWorkspacePath(path)
	if err != nil {
		return nil, err
	}
	if name == "" {
		folder := strings.TrimRight(path, `/\`)
		name = folder[strings.LastIndexAny(folder, `/\`)+1:]
		if name == "" {
			name = path
		}
	}
	workspace := chat.WorkSpace{ID: newID(), Name: name, Path: path}
	index.Workspaces = append(index.Workspaces, workspace)
	file, err := c.workspaceFile()
	if err != nil {
		return nil, err
	}
	if err := c.writeJSONLocked(file, index); err != nil {
		return nil, fmt.Errorf("保存工作区失败：%w", err)
	}
	return &workspace, nil
}

func (c *Core) RenameWorkspace(id, name string) (*chat.WorkSpace, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("工作区名称不能为空")
	}
	index, err := c.readWorkspacesLocked()
	if err != nil {
		return nil, err
	}
	for i := range index.Workspaces {
		if index.Workspaces[i].ID != id {
			continue
		}
		index.Workspaces[i].Name = name
		file, err := c.workspaceFile()
		if err != nil {
			return nil, err
		}
		if err := c.writeJSONLocked(file, index); err != nil {
			return nil, fmt.Errorf("保存工作区名称失败：%w", err)
		}
		return &index.Workspaces[i], nil
	}
	return nil, fmt.Errorf("工作区 %q 不存在", id)
}

// DeleteWorkspace 移除工作区记录，所属对话改为不使用工作区，保留文件夹和对话。
func (c *Core) DeleteWorkspace(id string) error {
	return c.deleteWorkspace(id, false)
}

// DeleteWorkspaceAndChats 删除工作区记录及所属对话，保留工作区文件夹。
func (c *Core) DeleteWorkspaceAndChats(id string) error {
	return c.deleteWorkspace(id, true)
}

func (c *Core) deleteWorkspace(id string, deleteChats bool) error {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	index, err := c.readWorkspacesLocked()
	if err != nil {
		return err
	}
	position := -1
	for i := range index.Workspaces {
		if index.Workspaces[i].ID == id {
			position = i
			break
		}
	}
	if position < 0 {
		return fmt.Errorf("工作区 %q 不存在", id)
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return err
	}
	chats, err := c.readChatIndexLocked(directory)
	if err != nil {
		return err
	}
	var sessions []*chat.ChatTask
	for _, summary := range chats.Chats {
		if summary.WorkspaceID != id {
			continue
		}
		if c.chatSending[summary.ID] {
			return errors.New("工作区内有对话正在生成中，暂时无法删除")
		}
		session, err := c.readChatLocked(directory, summary.ID)
		if deleteChats && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		sessions = append(sessions, session)
	}
	if deleteChats {
		for _, session := range sessions {
			if err := c.FS.DeleteFile(filepath.Join(directory, session.ID+".json")); err != nil {
				return fmt.Errorf("删除工作区对话失败：%w", err)
			}
			delete(c.ChatList, session.ID)
		}
		remaining := chats.Chats[:0]
		for _, summary := range chats.Chats {
			if summary.WorkspaceID != id {
				remaining = append(remaining, summary)
			}
		}
		chats.Chats = remaining
		if err := c.writeJSONLocked(filepath.Join(directory, "index.json"), chats); err != nil {
			return err
		}
	} else {
		// 先解除对话关联，全部保存成功后再移除工作区记录。
		for _, session := range sessions {
			session.WorkspaceID = ""
			if err := c.saveChatLocked(directory, session); err != nil {
				return err
			}
			delete(c.ChatList, session.ID)
		}
	}
	index.Workspaces = append(index.Workspaces[:position], index.Workspaces[position+1:]...)
	file, err := c.workspaceFile()
	if err != nil {
		return err
	}
	if err := c.writeJSONLocked(file, index); err != nil {
		return fmt.Errorf("删除工作区记录失败：%w", err)
	}
	return nil
}

func (c *Core) workspaceByIDLocked(id string) (*chat.WorkSpace, error) {
	if id == "" {
		return nil, nil
	}
	index, err := c.readWorkspacesLocked()
	if err != nil {
		return nil, err
	}
	for _, workspace := range index.Workspaces {
		if workspace.ID == id {
			return &workspace, nil
		}
	}
	return nil, fmt.Errorf("工作区 %q 不存在", id)
}

// workspaceID 为空表示不使用工作区，不更改对话权限档位。
func (c *Core) SetChatWorkspace(id, workspaceID string) (*chat.ChatTask, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSending[id] {
		return nil, errors.New("对话正在生成中，暂时无法切换工作区")
	}
	workspace, err := c.workspaceByIDLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	if workspace != nil {
		if _, err := c.validateWorkspacePath(workspace.Path); err != nil {
			return nil, err
		}
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	session.WorkspaceID = workspaceID
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Core) toolContextFor(session *chat.ChatTask) (*agenttool.ToolContext, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	workspace, err := c.workspaceByIDLocked(session.WorkspaceID)
	if err != nil {
		return nil, err
	}
	path := ""
	if workspace != nil {
		path, err = c.validateWorkspacePath(workspace.Path)
		if err != nil {
			return nil, err
		}
	}
	permission := agenttool.PermissionAutoEdit
	if c.Configs != nil && c.Configs.ToolNormalConfig != nil && c.Configs.ToolNormalConfig.Permission != 0 {
		permission = c.Configs.ToolNormalConfig.Permission
	}
	if session.ToolConfig != nil && session.ToolConfig.Permission != 0 {
		permission = session.ToolConfig.Permission
	}
	if permission < agenttool.PermissionReadOnly || permission > agenttool.PermissionFullAccess {
		return nil, errors.New("无效的工具权限配置")
	}
	return agenttool.CreateContext(path, permission), nil
}
