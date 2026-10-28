package apiImpl

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

// 表示一种模型
type Model struct {
	Name              string
	MaxContext        int
	Max_output_tokens int
	Input_modalities  []string
	output_modalities []string
	SupportedLevels   []string
}
