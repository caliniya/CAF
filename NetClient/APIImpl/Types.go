package apiImpl

import "encoding/json"

// ChatMessage 是一条对话消息。
type ChatMessage struct {
	Role string // "system" | "user" | "assistant" | "tool"

	Content string

	// 思考内容：支持思考的模型会分两路输出，
	Reasoning string

	// assistant 发起的工具调用；一条消息可能同时携带 Content 和多个 ToolCalls。
	ToolCalls []ToolCall

	// Role == "tool" 时有效：本次结果对应哪一次调用（与 ToolCall.ID 对齐）。
	ToolCallID string
}

// ToolCall 是一次工具调用。Arguments 是 JSON 对象的字符串形式
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Tool 是提供给模型的工具声明
// 原样进入请求体，由模型决定是否调用。
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ChatRequest 是一次对话请求（协议无关）。
// MaxTokens/Temperature 为零值时表示不发送该字段。
type ChatRequest struct {
	Model     string
	Messages  []ChatMessage
	Tools     []Tool
	Thinking  string
	MaxTokens int

	//通常来说这条没用
	Temperature float64
}

type Delta struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCall
	Usage     *TokenUsage
	Done      bool
}

// TokenUsage 是模型服务返回的本次请求 token 用量。
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamError 表示服务端在流中返回的错误对象。
type StreamError struct {
	Message string
	Code    any
}

func (e *StreamError) Error() string {
	if e.Code != nil {
		return "模型服务错误: " + e.Message
	}
	return "模型服务错误: " + e.Message
}

// Model 表示模型服务返回的模型元数据。
type Model struct {
	ID               string   `json:"id"`
	Name             string   `json:"name,omitempty"`
	MaxContext       int      `json:"context_window,omitempty"`
	MaxOutputTokens  int      `json:"max_output_tokens,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	SupportedLevels  []string `json:"supported_levels,omitempty"`
	DefaultLevel     string   `json:"default_level,omitempty"`
}

func (m *Model) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		ContextWindow    int      `json:"context_window"`
		MaxOutputTokens  int      `json:"max_output_tokens"`
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
		SupportedLevels  []string `json:"supported_levels"`
		DefaultLevel     string   `json:"default_level"`
		Effort           struct {
			SupportedLevels []string `json:"supported_levels"`
			DefaultLevel    string   `json:"default_level"`
		} `json:"effort"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.ID = raw.ID
	m.Name = raw.Name
	m.MaxContext = raw.ContextWindow
	m.MaxOutputTokens = raw.MaxOutputTokens
	m.InputModalities = raw.InputModalities
	m.OutputModalities = raw.OutputModalities
	m.SupportedLevels = raw.SupportedLevels
	m.DefaultLevel = raw.DefaultLevel
	if len(m.SupportedLevels) == 0 {
		m.SupportedLevels = raw.Effort.SupportedLevels
	}
	if m.DefaultLevel == "" {
		m.DefaultLevel = raw.Effort.DefaultLevel
	}
	return nil
}
