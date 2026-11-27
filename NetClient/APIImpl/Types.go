package APIImpl

import "encoding/json"

// ChatMessage 是一条对话消息。
type ChatMessage struct {
	Role string `json:"role"` // "system" | "user" | "assistant" | "tool"

	Content     string       `json:"content,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"` // user-provided references; image bytes live outside the tree
	Images      []InputImage `json:"-"`                     // request only, for the current multimodal request
	// Tokens 只记录这条消息新增的用量，用于粗略划分压缩范围；协议不会发送此字段。
	// 模型输出优先使用本次 completion_tokens，用户消息和工具结果使用文本估算。
	Tokens int `json:"tokens,omitempty"`

	// 思考内容：支持思考的模型会分两路输出，
	Reasoning string `json:"reasoning,omitempty"`

	// assistant 发起的工具调用；一条消息可能同时携带 Content 和多个 ToolCalls。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// Role == "tool" 时有效：本次结果对应哪一次调用（与 ToolCall.ID 对齐）。
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolError  string `json:"tool_error,omitempty"` // 仅用于界面和历史记录。
}

// ToolCall 是一次工具调用。Arguments 是 JSON 对象的字符串形式
type ToolCall struct {
	Image          *Attachment   `json:"image,omitempty"` // snapshot taken by ReadImage
	ID             string        `json:"id"`              // 本次调用的 ID，用于关联工具结果。
	Name           string        `json:"name"`            // API 函数名，即 ToolMeta.ID。
	Arguments      string        `json:"arguments"`
	ImageSent      bool          `json:"image_sent,omitempty"`   // upstream responded to the request containing the pixels
	DisplayName    string        `json:"display_name,omitempty"` // 仅用于界面和历史记录，不发送给模型。
	DisplayType    string        `json:"display_type,omitempty"` // 展示字段不发送给模型。
	DisplayContent string        `json:"display_content,omitempty"`
	Approval       *ToolApproval `json:"approval,omitempty"`
}

// Tool 是提供给模型的工具声明
// 原样进入请求体，由模型决定是否调用。
type Tool struct {
	Name        string         `json:"name"` // API 函数名，即 ToolMeta.ID。
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ChatRequest 是一次对话请求（协议无关）。
// MaxTokens/Temperature 为零值时表示不发送该字段。
type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	Tools     []Tool        `json:"tools,omitempty"`
	Thinking  string        `json:"thinking,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`

	//通常来说这条没用
	Temperature float64 `json:"temperature,omitempty"`
}

type Delta struct {
	Content   string      `json:"content,omitempty"`
	Reasoning string      `json:"reasoning,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
	Usage     *TokenUsage `json:"usage,omitempty"`
	Done      bool        `json:"done,omitempty"`
}

// TokenUsage 是模型服务返回的本次请求 token 用量。
type TokenUsage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	CompletionTokens        int                     `json:"completion_tokens"`
	TotalTokens             int                     `json:"total_tokens"`
	PromptTokensDetails     *PromptTokenDetails     `json:"prompt_tokens_details,omitempty"`
	PromptCacheHitTokens    *int                    `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens   *int                    `json:"prompt_cache_miss_tokens,omitempty"`
	CompletionTokensDetails *CompletionTokenDetails `json:"completion_tokens_details,omitempty"`
}

type PromptTokenDetails struct {
	// nil 表示未上报缓存用量，明确返回 0 才表示没有命中。
	CachedTokens *int `json:"cached_tokens,omitempty"`
}

type CompletionTokenDetails struct {
	// nil 表示服务端没有提供统计，区别于明确返回 0。
	ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
}

// StreamError 表示服务端在流中返回的错误对象。
type StreamError struct {
	Message string `json:"message"`
	Code    any    `json:"code,omitempty"`
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
