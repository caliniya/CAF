package apiImpl

// ChatCompletionsRequest 表示 Chat Completions API 的请求体。
type ChatCompletionsRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Stream     bool          `json:"stream,omitempty"`
	MaxTokens  int           `json:"max_tokens,omitempty"`
	Tools      []ChatTool    `json:"tools,omitempty"`
	ToolChoice any           `json:"tool_choice,omitempty"`
}

// ChatMessage 表示对话中的一条消息，也可承载模型的工具调用或工具执行结果。
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatTool 声明模型可以调用的工具。目前 DeepSeek Chat Completions 支持 function 类型。
type ChatTool struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

// FunctionSpec 描述函数用途和参数 JSON Schema。
type FunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

// ToolCall 是 assistant 消息中的一次函数调用。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 包含模型生成的函数名称和 JSON 格式参数字符串。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// NamedToolChoice 可用于强制模型调用指定函数。
type NamedToolChoice struct {
	Type     string              `json:"type"`
	Function NamedFunctionChoice `json:"function"`
}

// NamedFunctionChoice 指定要调用的函数名称。
type NamedFunctionChoice struct {
	Name string `json:"name"`
}
