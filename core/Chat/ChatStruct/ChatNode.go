package chatstruct

//多轮对话的一个节点,可以是发送的消息,模型回复,或者过程
//对于过程中节点,以子节点的形式存在(不是下一节点)
type ChatNode struct {
	//此轮的文本,对于包含子节点的这是开头
	Text string

	//下一级所有的节点
	Next *[]ChatNode

	//使用的下一个节点
	Choice int

	//一个过程中的子节点
	Child *[]ChatNode
}

//0表示用户消息,2表示工具消息,3表示模型回复,4表示系统提示词
var ChatType []string = []string{
	"chat",
	"tool",
	"assistant",
	"system",
}
