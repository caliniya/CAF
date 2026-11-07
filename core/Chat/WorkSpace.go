package Chat

// 表示一个工作区
type WorkSpace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	//一个工作区当然可以有很多任务
	// 对话通过 WorkspaceID 关联，列表不重复保存对话内容。
	Tasks *[]ChatTask `json:"-"`
}
