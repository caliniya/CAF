package chatstruct

import (
	"errors"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// ChatNode 保存一段消息以及可选的后续对话分支。
type ChatNode struct {
	// Choice 是当前选中的后续分支下标。
	Choice int
	// Next 保存所有可选的后续分支。
	Next []*ChatNode
	// Child 保存当前节点内按顺序发生的消息。
	Child []apiImpl.ChatMessage
}

// GetEach 按当前选中的分支顺序访问消息。
func (n *ChatNode) GetEach(get func(*apiImpl.ChatMessage)) {
	if n == nil || get == nil {
		return
	}
	_ = n.walkCurrentPath(func(message *apiImpl.ChatMessage) error {
		get(message)
		return nil
	}, make(map[*ChatNode]struct{}))
}

func CreateChatNode(messages ...apiImpl.ChatMessage) *ChatNode {
	return &ChatNode{Child: append([]apiImpl.ChatMessage(nil), messages...)}
}

// AddMessage 将一条消息追加到当前节点。
func (n *ChatNode) AddMessage(message apiImpl.ChatMessage) {
	if n == nil {
		return
	}
	n.Child = append(n.Child, message)
}

// AddNext 添加一个分支，并将新分支设为当前选中分支。nil 节点会被忽略。
func (n *ChatNode) AddNext(node *ChatNode) int {
	if n == nil || node == nil {
		return -1
	}
	n.Next = append(n.Next, node)
	n.Choice = len(n.Next) - 1
	return n.Choice
}

// SelectNext 选择一个已有的后续分支。
func (n *ChatNode) SelectNext(index int) error {
	if n == nil {
		return errors.New("chat node is nil")
	}
	if index < 0 || index >= len(n.Next) {
		return errors.New("branch index out of range")
	}
	n.Choice = index
	return nil
}

// GetNext 返回当前选中的后续分支；没有有效分支时返回 nil。
func (n *ChatNode) GetNext() *ChatNode {
	if n == nil || n.Choice < 0 || n.Choice >= len(n.Next) {
		return nil
	}
	return n.Next[n.Choice]
}

// GetMessages 返回从当前节点开始、沿每个节点所选分支形成的消息历史。
func (n *ChatNode) GetMessages() ([]apiImpl.ChatMessage, error) {
	messages := make([]apiImpl.ChatMessage, 0)
	err := n.walkCurrentPath(func(message *apiImpl.ChatMessage) error {
		messages = append(messages, *message)
		return nil
	}, make(map[*ChatNode]struct{}))
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func (n *ChatNode) walkCurrentPath(visit func(*apiImpl.ChatMessage) error, seen map[*ChatNode]struct{}) error {
	if n == nil {
		return nil
	}
	if _, exists := seen[n]; exists {
		return errors.New("cycle detected in chat tree")
	}
	seen[n] = struct{}{}
	defer delete(seen, n)

	for i := range n.Child {
		if err := visit(&n.Child[i]); err != nil {
			return err
		}
	}
	if len(n.Next) == 0 {
		return nil
	}
	if n.Choice < 0 || n.Choice >= len(n.Next) {
		return errors.New("selected branch index out of range")
	}
	return n.Next[n.Choice].walkCurrentPath(visit, seen)
}
