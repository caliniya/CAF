package chatstruct

import (
	"errors"
	"fmt"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// 对话树,树形结构是我可以想到的比较好的思路了
type ChatTree struct {
	Root *TreeRoot
}

// 树的抽象根节点,不是对话的一部分
type TreeRoot struct {
	Choice int
	Next   []*ChatNode
	Name   string
}

// 将当前的树结构转为请求(使用当前的分支结构)
// 注意,一些字段是没有设置的(比如api地址)
func (tree *ChatTree) ToChatRequest() *apiImpl.ChatRequest {
	return &apiImpl.ChatRequest{}
}

// 自然是用户开始的所以说只有一条
func (tree *ChatTree) Start(text apiImpl.ChatMessage) *ChatNode {
	n := &ChatNode{Child: []apiImpl.ChatMessage{text}, Choice: 0}
	tree.Root.Add(n)
	return n
}

func (r *TreeRoot) Add(node *ChatNode) {
	r.Next = append(r.Next, node)
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

func CreateTree(name string) *ChatTree {
	return &ChatTree{
		Root: &TreeRoot{
			Name: name,
		}}
}
