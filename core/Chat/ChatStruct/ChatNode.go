package chatstruct

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// ChatNode 保存一段消息以及可选的后续对话分支。
type ChatNode struct {
	ID     string                `json:"id"`
	Choice int                   `json:"choice"`
	Next   []*ChatNode           `json:"next"`
	Child  []apiImpl.ChatMessage `json:"child"`
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
	return &ChatNode{ID: newNodeID(), Child: append([]apiImpl.ChatMessage(nil), messages...), Next: []*ChatNode{}}
}

func newNodeID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err == nil {
		return hex.EncodeToString(id[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
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

// BranchIndex 返回 node 在当前节点后续分支中的下标。
func (n *ChatNode) BranchIndex(node *ChatNode) int {
	if n == nil || node == nil {
		return -1
	}
	for i, candidate := range n.Next {
		if candidate == node {
			return i
		}
	}
	return -1
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

// MessagesThrough 返回当前所选路径中截至 target（包含 target）的消息。
func (n *ChatNode) MessagesThrough(target *ChatNode) ([]apiImpl.ChatMessage, error) {
	messages := make([]apiImpl.ChatMessage, 0)
	found := false
	var walk func(*ChatNode, map[*ChatNode]struct{}) error
	walk = func(node *ChatNode, seen map[*ChatNode]struct{}) error {
		if node == nil || found {
			return nil
		}
		if _, exists := seen[node]; exists {
			return errors.New("cycle detected in chat tree")
		}
		seen[node] = struct{}{}
		defer delete(seen, node)
		messages = append(messages, node.Child...)
		if node == target {
			found = true
			return nil
		}
		if len(node.Next) == 0 {
			return nil
		}
		if node.Choice < 0 || node.Choice >= len(node.Next) {
			return errors.New("selected branch index out of range")
		}
		return walk(node.Next[node.Choice], seen)
	}
	if err := walk(n, make(map[*ChatNode]struct{})); err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("node is not on the selected path")
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
