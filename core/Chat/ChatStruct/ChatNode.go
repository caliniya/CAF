package chatstruct

import apiImpl "github.com/caliniya/caf/NetClient/APIImpl"

//多轮对话的一个节点一般来说用户消息只有一条,模型回复会有多个
//对于过程中节点,以子节点的形式存在(不是下一节点)
type ChatNode struct {

	//使用的下一个节点数组下标
	Choice int
	//下一级所有的节点
	Next []ChatNode

	//一个对话的过程(原则上是包含工具调用过程的),也就是直到模型完成一次用户工作
	Child []apiImpl.ChatMessage
}

//递归的获取所有消息
func (n *ChatNode) GetEach(get func(*apiImpl.ChatMessage)) {
	if n == nil || get == nil {
		return
	}

	for i := range n.Child {
		get(&n.Child[i])
	}

	if n.Choice >= 0 && n.Choice < len(n.Next) {
		n.Next[n.Choice].GetEach(get)
	}
}

func (n *ChatNode) CreateChatNode(m ...apiImpl.ChatMessage) *ChatNode {
	a := &ChatNode{Child: m, Next: []ChatNode{}}
	return a
}

func (n *ChatNode) GetNext() *ChatNode {
	return &n.Next[n.Choice]
}

func (n *ChatNode) AddNext(node *ChatNode) {
	n.Next = append(n.Next, *node)
}
