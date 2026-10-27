package chatstruct

import apiImpl "github.com/caliniya/caf/NetClient/APIImpl"

//多轮对话的一个节点一般来说用户消息只有一条,模型回复会有多个
//对于过程中节点,以子节点的形式存在(不是下一节点)
type ChatNode struct {

	//使用的下一个节点索引,在go里面是从0开始的
	Choice int

	//下一级所有的节点
	Next []ChatNode

	//一个对话的过程
	Child []apiImpl.ChatMessage
}

//递归的获取所有消息
func (n *ChatNode) GetEach(get func(res *apiImpl.ChatMessage)) {
	for i := range n.Child {
		get(&n.Child[i])
	}
}

func (n *ChatNode) CreateNext(m ...apiImpl.ChatMessage) *ChatNode {
	a := &ChatNode{Child: m}
	return a
}
