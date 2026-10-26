package chatstruct

import apiImpl "github.com/caliniya/caf/NetClient/APIImpl"

//对话树,树形结构是我可以想到的比较好的思路了
type ChatTree struct {
	Root *TreeRoot
}

//根节点配置名字和子节点就够了?
type TreeRoot struct {
	Next  *[]ChatNode
	Model string
	Name  string
}

//将当前的树结构转为请求
//注意,一些字段是没有设置的(比如api地址)
func (tree *ChatTree) ToChatRequest() *apiImpl.ChatRequest {
	return &apiImpl.ChatRequest{}
}
