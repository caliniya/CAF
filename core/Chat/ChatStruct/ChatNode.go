package chatstruct

//多轮对话的一个节点,可以是发送的消息,模型回复,或者过程
//对于过程中节点,以子节点的形式存在
type ChatNode struct {
	Text string
	//下一级所有的节点
	Next *[]ChatNode
	//使用的下一个节点
	Choice int
}
