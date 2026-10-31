package chatstruct

import (
	"errors"
	"fmt"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// 对话树,树形结构是我可以想到的比较好的思路了
type ChatTree struct {
	Root *TreeRoot `json:"root"`
}

// 树的抽象根节点,不是对话的一部分
type TreeRoot struct {
	Choice int         `json:"choice"`
	Next   []*ChatNode `json:"next"`
	Name   string      `json:"name"`
}

// ToChatRequest 将当前分支转换为模型请求。
func (tree *ChatTree) ToChatRequest(model string) (*apiImpl.ChatRequest, error) {
	if tree == nil || tree.Root == nil {
		return nil, errors.New("对话树尚未初始化")
	}
	messages := make([]apiImpl.ChatMessage, 0)
	err := tree.Each(func(message *apiImpl.ChatMessage) {
		messages = append(messages, *message)
	})
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, errors.New("对话树没有消息")
	}
	return &apiImpl.ChatRequest{Model: model, Messages: messages}, nil
}

// Start 在根节点下创建并选中一条新的对话起始分支。
func (tree *ChatTree) Start(text apiImpl.ChatMessage) *ChatNode {
	if tree == nil {
		return nil
	}
	if tree.Root == nil {
		tree.Root = &TreeRoot{}
	}
	n := CreateChatNode(text)
	tree.Root.Add(n)
	return n
}

func (r *TreeRoot) Add(node *ChatNode) int {
	if r == nil || node == nil {
		return -1
	}
	r.Next = append(r.Next, node)
	r.Choice = len(r.Next) - 1
	return r.Choice
}

func (tree *ChatTree) Each(get func(*apiImpl.ChatMessage)) error {
	if tree == nil || tree.Root == nil {
		return errors.New("对话树尚未初始化")
	}
	if get == nil {
		return errors.New("消息回调不能为空")
	}

	root := tree.Root

	// 空树没有消息，不需要继续遍历。
	if len(root.Next) == 0 {
		return nil
	}

	// 检查根节点选择的分支是否存在。
	if root.Choice < 0 || root.Choice >= len(root.Next) {
		return fmt.Errorf(
			"根节点分支下标越界：%d（共 %d 个分支）",
			root.Choice,
			len(root.Next),
		)
	}

	node := root.Next[root.Choice]
	if node == nil {
		return errors.New("根节点选中的分支为空")
	}

	// 复用节点已有的内部遍历方法。
	// seen 用来记录访问过的节点，避免出现环时无限递归。
	return node.walkCurrentPath(
		func(message *apiImpl.ChatMessage) error {
			get(message)
			return nil
		},
		make(map[*ChatNode]struct{}),
	)
}

// AddNext 在指定节点（空 ID 表示抽象根）下添加并选中新分支。
func (tree *ChatTree) AddNext(parentID string, node *ChatNode) error {
	if tree == nil || tree.Root == nil || node == nil {
		return errors.New("对话树或新节点无效")
	}
	if parentID == "" {
		tree.Root.Add(node)
		return nil
	}
	parent := tree.FindNode(parentID)
	if parent == nil {
		return fmt.Errorf("未找到父节点 %q", parentID)
	}
	if parent.AddNext(node) < 0 {
		return errors.New("添加分支失败")
	}
	return nil
}

// SelectBranch 设置指定节点（空 ID 表示抽象根）当前选中的后续分支。
func (tree *ChatTree) SelectBranch(parentID string, choice int) error {
	if tree == nil || tree.Root == nil {
		return errors.New("对话树尚未初始化")
	}
	if parentID == "" {
		if choice < 0 || choice >= len(tree.Root.Next) {
			return errors.New("根节点分支下标越界")
		}
		if tree.Root.Next[choice] == nil {
			return errors.New("根节点分支为空")
		}
		tree.Root.Choice = choice
		return nil
	}
	parent := tree.FindNode(parentID)
	if parent == nil {
		return fmt.Errorf("未找到节点 %q", parentID)
	}
	return parent.SelectNext(choice)
}

// FindNode 在整棵树（包括未选中分支）中按 ID 查找节点。
func (tree *ChatTree) FindNode(id string) *ChatNode {
	if tree == nil || tree.Root == nil || id == "" {
		return nil
	}
	seen := make(map[*ChatNode]struct{})
	var find func(*ChatNode) *ChatNode
	find = func(node *ChatNode) *ChatNode {
		if node == nil {
			return nil
		}
		if _, ok := seen[node]; ok {
			return nil
		}
		seen[node] = struct{}{}
		if node.ID == id {
			return node
		}
		for _, next := range node.Next {
			if result := find(next); result != nil {
				return result
			}
		}
		return nil
	}
	for _, node := range tree.Root.Next {
		if result := find(node); result != nil {
			return result
		}
	}
	return nil
}

// CurrentPath 返回抽象根下沿当前 Choice 选中的节点路径。
func (tree *ChatTree) CurrentPath() ([]*ChatNode, error) {
	if tree == nil || tree.Root == nil {
		return nil, errors.New("对话树尚未初始化")
	}
	path := make([]*ChatNode, 0)
	if len(tree.Root.Next) == 0 {
		return path, nil
	}
	if tree.Root.Choice < 0 || tree.Root.Choice >= len(tree.Root.Next) {
		return nil, errors.New("根节点分支下标越界")
	}
	seen := make(map[*ChatNode]struct{})
	node := tree.Root.Next[tree.Root.Choice]
	for node != nil {
		if _, ok := seen[node]; ok {
			return nil, errors.New("cycle detected in chat tree")
		}
		seen[node] = struct{}{}
		path = append(path, node)
		if len(node.Next) == 0 {
			break
		}
		if node.Choice < 0 || node.Choice >= len(node.Next) {
			return nil, errors.New("selected branch index out of range")
		}
		node = node.Next[node.Choice]
	}
	return path, nil
}

// ParentOf 在整棵树中查找节点的直接父节点；抽象根的子节点返回 nil 父节点。
func (tree *ChatTree) ParentOf(id string) (*ChatNode, bool) {
	if tree == nil || tree.Root == nil || id == "" {
		return nil, false
	}
	seen := make(map[*ChatNode]struct{})
	var find func(*ChatNode) (*ChatNode, bool)
	find = func(parent *ChatNode) (*ChatNode, bool) {
		if parent == nil {
			return nil, false
		}
		if _, ok := seen[parent]; ok {
			return nil, false
		}
		seen[parent] = struct{}{}
		for _, child := range parent.Next {
			if child == nil {
				continue
			}
			if child.ID == id {
				return parent, true
			}
			if foundParent, found := find(child); found {
				return foundParent, true
			}
		}
		return nil, false
	}
	for _, node := range tree.Root.Next {
		if node != nil && node.ID == id {
			return nil, true
		}
		if parent, found := find(node); found {
			return parent, true
		}
	}
	return nil, false
}

// MessagesThrough 返回当前路径中截至指定节点（包含该节点）的完整消息历史。
func (tree *ChatTree) MessagesThrough(nodeID string) ([]apiImpl.ChatMessage, error) {
	if tree == nil || tree.Root == nil {
		return nil, errors.New("对话树尚未初始化")
	}
	if len(tree.Root.Next) == 0 {
		return nil, errors.New("对话树没有消息")
	}
	if tree.Root.Choice < 0 || tree.Root.Choice >= len(tree.Root.Next) {
		return nil, errors.New("根节点分支下标越界")
	}
	return tree.Root.Next[tree.Root.Choice].MessagesThrough(tree.FindNode(nodeID))
}

func CreateTree(name string) *ChatTree {
	return &ChatTree{
		Root: &TreeRoot{
			Name: name,
		}}
}
