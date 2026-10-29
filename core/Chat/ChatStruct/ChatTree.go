package chatstruct

import apiImpl "github.com/caliniya/caf/NetClient/APIImpl"

//对话树,树形结构是我可以想到的比较好的思路了
type ChatTree struct {
	Root *TreeRoot
}

//树的抽象根节点,不是对话的一部分
type TreeRoot struct {
	Choice int
	Next   []*ChatNode
	Model  string
	Name   string
}

//将当前的树结构转为请求(使用当前的分支结构)
//注意,一些字段是没有设置的(比如api地址)
func (tree *ChatTree) ToChatRequest() *apiImpl.ChatRequest {
	return &apiImpl.ChatRequest{}
}

//自然是用户开始的所以说只有一条
func (tree *ChatTree) Start(text apiImpl.ChatMessage) *ChatNode {
	n := &ChatNode{Child: []apiImpl.ChatMessage{text}, Choice: 0}
	//tree.Root
	return n
}

func (r *TreeRoot) Add(node *ChatNode) {
	r.Next = append(r.Next, node)
}
