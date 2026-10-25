package chat

//表示一个工作区
type WorkSpace struct {
	Name  string
	Path  string
	Tasks *[]ChatTask
}
