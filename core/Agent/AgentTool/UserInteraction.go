package AgentTool

// UserQuestion 支持建议选项和自由文本回答。
type UserQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}
