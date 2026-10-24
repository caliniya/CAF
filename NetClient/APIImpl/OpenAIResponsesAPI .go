package apiImpl

// OpenAIRequest 表示 OpenAI Responses API 的请求体。
type OpenAIRequest struct {
	Model string `json:"model"` // 要使用的模型名称。

	// Input 是提供给模型的输入，可以是字符串，也可以是消息项列表。
	Input any `json:"input,omitempty"`

	// Instructions 是对模型的系统级指令，例如回答风格或行为要求。
	Instructions string `json:"instructions,omitempty"`

	// Stream 为 true 时以 SSE 事件流的形式接收响应；省略或为 false 时等待完整响应。
	Stream bool `json:"stream,omitempty"`

	// MaxOutputTokens 限制响应最多生成的 token 数量；0 表示不在请求中发送该字段。
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`

	// Temperature 控制输出随机性，通常范围为 0 到 2；0 值会被 omitempty 省略。
	Temperature float64 `json:"temperature,omitempty"`

	// TopP 是另一种采样参数；通常与 Temperature 二选一调整。
	TopP float64 `json:"top_p,omitempty"`
}
