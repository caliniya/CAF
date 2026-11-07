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
	if name == "" {
		return nil, errors.New("工作区名称不能为空")
	}
	index, err := c.readWorkspacesLocked()
	if err != nil {
		return nil, err
	}
	path, err = c.validateWorkspacePath(path)
	if err != nil {
		return nil, err
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
