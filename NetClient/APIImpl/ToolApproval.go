package APIImpl

// ToolApproval 是宿主生成的审批记录，保存在对话中，协议转换不会把它发送给模型。
type ToolApproval struct {
	CallID           string `json:"callId"`
	ToolID           string `json:"toolId"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	Reason           string `json:"reason"`
	WorkspaceID      string `json:"workspaceId,omitempty"`
	WorkspaceName    string `json:"workspaceName,omitempty"`
	WorkspaceAllowed bool   `json:"workspaceAllowed"`
	HigherPermission int    `json:"higherPermission,omitempty"`
	Decision         string `json:"decision,omitempty"`
}
