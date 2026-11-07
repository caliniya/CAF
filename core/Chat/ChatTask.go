package Chat

import (
	"time"

	netclient "github.com/caliniya/caf/NetClient"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

type ChatTask struct {
	ID              string                    `json:"id"`
	Title           string                    `json:"title"`
	ConfigID        string                    `json:"configId"`
	Model           string                    `json:"model"`
	WorkspaceID     string                    `json:"workspaceId,omitempty"`
	ReasoningEffort string                    `json:"reasoningEffort,omitempty"`
	Tree            *chatstruct.ChatTree      `json:"tree"`
	CreatedAt       time.Time                 `json:"createdAt"`
	UpdatedAt       time.Time                 `json:"updatedAt"`
	Client          *netclient.APIClient      `json:"-"`
	ToolConfig      *agenttool.ChatToolConfig `json:"toolConfig,omitempty"`
}

type ChatSummary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ConfigID    string    `json:"configId"`
	Model       string    `json:"model"`
	WorkspaceID string    `json:"workspaceId,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func NewChatTask(id, title, configID, model string, tree *chatstruct.ChatTree) *ChatTask {
	if tree == nil {
		tree = chatstruct.CreateTree(title)
	}
	now := time.Now().UTC()
	return &ChatTask{
		ID: id, Title: title, ConfigID: configID, Model: model,
		ReasoningEffort: "high",
		Tree:            tree, CreatedAt: now, UpdatedAt: now,
	}
}

func CreateChatTask(id, title, configID, model string, tree *chatstruct.ChatTree) *ChatTask {
	return NewChatTask(id, title, configID, model, tree)
}
