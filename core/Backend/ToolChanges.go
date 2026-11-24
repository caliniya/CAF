package Backend

import (
	"errors"
	"fmt"
	"strings"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	texts "github.com/caliniya/caf/utils/text"
)

// SetChatToolChangesUndone 按整轮回复撤销或恢复已完成的工具改动。
// 撤销从后往前，恢复从前往后；文件内容使用保存的副本，不重新执行工具。
func (c *Core) SetChatToolChangesUndone(id, nodeID string, undone bool) (*chat.ChatTask, error) {
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	server.toolMu.Lock()
	defer server.toolMu.Unlock()
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.sending[id] != nil {
		return nil, errors.New("对话正在生成中，暂时无法撤销或恢复改动")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(directory, id)
	if err != nil {
		return nil, err
	}
	path, err := session.Tree.CurrentPath()
	if err != nil {
		return nil, err
	}
	position := -1
	for i, node := range path {
		if node.ID == nodeID {
			position = i
			break
		}
	}
	if position < 0 || len(path[position].ToolOperations) == 0 {
		return nil, errors.New("当前分支中未找到这轮回复的可撤销改动")
	}
	node := path[position]
	state := node.ToolOperations[0].State
	if state == agenttool.ToolOperationDiscarded {
		return nil, errors.New("撤销后已产生新的改动，无法取消此次撤销")
	}
	if state != "" && state != agenttool.ToolOperationUndone {
		return nil, errors.New("工具改动状态无效")
	}
	for _, operation := range node.ToolOperations {
		if operation.Pending || operation.State != state {
			return nil, errors.New("这轮回复的工具改动尚未完成或状态不一致")
		}
	}
	nextState := ""
	if undone {
		nextState = agenttool.ToolOperationUndone
	}
	if state == nextState {
		return session, nil
	}
	for i, other := range path {
		for _, operation := range other.ToolOperations {
			if i > position && operation.State == "" {
				return nil, errors.New("请先撤销后续回复的改动")
			}
			if !undone && i < position && operation.State == agenttool.ToolOperationUndone {
				return nil, errors.New("请先取消此前回复的撤销")
			}
		}
	}
	before := pathToolOperations(path)
	for i := range node.ToolOperations {
		node.ToolOperations[i].State = nextState
	}
	after := pathToolOperations(path)
	restoreState := func() {
		for i := range node.ToolOperations {
			node.ToolOperations[i].State = state
		}
	}
	if err := c.transitionToolOperations(before, after); err != nil {
		restoreState()
		return nil, err
	}
	previousUpdatedAt := session.UpdatedAt
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(directory, session); err != nil {
		restoreState()
		session.UpdatedAt = previousUpdatedAt
		return nil, errors.Join(err, c.transitionToolOperations(after, before), c.saveChatLocked(directory, session))
	}
	return session, nil
}

func toolChangesNotice(session *chat.ChatTask) (string, error) {
	path, err := session.Tree.CurrentPath()
	if err != nil {
		return "", err
	}
	var calls []string
	for _, node := range path {
		for _, operation := range node.ToolOperations {
			if operation.State == agenttool.ToolOperationUndone || operation.State == agenttool.ToolOperationDiscarded {
				calls = append(calls, operation.ToolID+" ("+operation.CallID+")")
			}
		}
	}
	if len(calls) == 0 {
		return "", nil
	}
	return fmt.Sprintf(texts.ToolChangesUndone, strings.Join(calls, ", ")), nil
}
