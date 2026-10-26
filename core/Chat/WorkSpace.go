package chat

//表示一个工作区
type WorkSpace struct {
	//显而易见
	Name string
	Path string
	//一个工作区当然可以有很多任务
	Tasks *[]ChatTask
}
