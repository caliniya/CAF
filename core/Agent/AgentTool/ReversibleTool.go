package AgentTool

// ReversibleTool 的恢复数据由工具自己保存和解释，宿主只记录文件位置。
// Restore 撤回本次操作，Reapply 恢复本次操作完成后的状态；均可重复调用。
// 实现必须先成功保存 RecoveryFile，才能修改目标。
type ReversibleTool interface {
	Tool
	Restore(*ToolContext, string) error
	Reapply(*ToolContext, string) error
}

// ToolOperation 关联历史调用和恢复文件；文件内容不进入模型上下文。
type ToolOperation struct {
	ID          string `json:"id"`
	ToolID      string `json:"toolId"`
	CallID      string `json:"callId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	// Workspace 保留执行时的路径，恢复不受对话后来切换工作区的影响。
	Workspace string `json:"workspace,omitempty"`
	// Pending 区分保存关联后尚未产生恢复文件的调用与已经完成的调用。
	Pending bool `json:"pending,omitempty"`
}
