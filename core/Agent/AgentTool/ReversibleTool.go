package AgentTool

// ReversibleTool 的恢复数据由工具自己保存和解释。
// Restore 撤回本次操作，Reapply 恢复本次操作完成后的状态；均可重复调用。
// Changes 从恢复文件提取实际改动摘要，供界面展示，不读取当前目标状态。
// 实现必须先成功保存 RecoveryFile，才能修改目标。
type ReversibleTool interface {
	Tool
	Changes(*ToolContext, string) ([]ToolChange, error)
	Restore(*ToolContext, string) error
	Reapply(*ToolContext, string) error
}

// ToolChange 不包含文件内容；文本改动按增删行统计，二进制改动按字节展示。
type ToolChange struct {
	Target       string `json:"target"`
	Action       string `json:"action"` // create、modify 或 delete。
	AddedLines   int    `json:"addedLines,omitempty"`
	RemovedLines int    `json:"removedLines,omitempty"`
	BeforeBytes  int    `json:"beforeBytes"`
	AfterBytes   int    `json:"afterBytes"`
	Binary       bool   `json:"binary,omitempty"`
	Approximate  bool   `json:"approximate,omitempty"`
}

const (
	ToolOperationUndone    = "undone"
	ToolOperationDiscarded = "discarded"
)

// ToolOperation 关联历史调用和恢复文件；文件内容不进入模型上下文。
type ToolOperation struct {
	ID          string `json:"id"`
	ToolID      string `json:"toolId"`
	CallID      string `json:"callId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	// Workspace 保留执行时的路径，恢复不受对话后来切换工作区的影响。
	Workspace string `json:"workspace,omitempty"`
	// Pending 区分保存关联后尚未产生恢复文件的调用与已经完成的调用。
	Pending bool         `json:"pending,omitempty"`
	Changes []ToolChange `json:"changes,omitempty"`
	// 空值表示已应用；撤销后继续产生新改动会丢弃原来的重做分支。
	State string `json:"state,omitempty"`
}
