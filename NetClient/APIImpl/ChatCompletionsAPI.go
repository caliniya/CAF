package apiImpl

// ChatCompletionsRequest 表示 Chat Completions API 的 JSON 请求体。
type ChatCompletionsRequest struct {
	// Model 是模型 ID，必填。可用值以账号当前可调用的模型列表和官方文档为准，
	// 例如文档中出现过 deepseek-flash、deepseek-v4-pro；模型名称可能随服务更新。
	Model string `json:"model"`

	// Messages 是对话历史，必填且至少包含一条消息。多轮对话时由调用方传入历史消息。
	Messages []ChatMessage `json:"messages"`

	// Stream 控制是否流式返回：true 表示通过 SSE 返回增量，结束标记为 data: [DONE]；
	// false 或省略表示等待完整 JSON 响应。默认值为 false。
	Stream bool `json:"stream,omitempty"`

	// MaxTokens 限制本次生成的最大 token 数，必须为正数；省略时由服务端使用模型默认值。
	// 输入与输出 token 总量仍受模型上下文长度限制。
	MaxTokens int `json:"max_tokens,omitempty"`

	// Tools 声明模型可调用的工具。DeepSeek Chat Completions 当前支持 type 为 "function"；
	// 不需要工具调用时可省略。模型只返回调用请求，实际执行函数需要由客户端完成。
	Tools []ChatTool `json:"tools,omitempty"`

	// ToolChoice 控制工具调用策略，可传 "none"、"auto"、"required"，
	// 或 NamedToolChoice 指定一个函数。未提供 tools 时默认相当于 "none"，
	// 提供 tools 时默认相当于 "auto"。部分思考模式下不支持 required 或指定函数。
	ToolChoice any `json:"tool_choice,omitempty"`
}

// ChatMessage 表示对话中的一条消息，也可承载模型的工具调用或工具执行结果。
type ChatMessage struct {
	// Role 表示消息角色，常见可用值为 "system"、"user"、"assistant"、"tool"。
	// tool 角色用于回传工具执行结果；assistant 消息可包含 ToolCalls。
	Role string `json:"role"`

	// Content 是消息文本；普通文本对话传 string。工具调用的 assistant 消息中可省略或为 null。
	Content any `json:"content,omitempty"`

	// Name 可选，用于标识消息参与者或工具名称；普通 user/assistant 消息通常省略。
	Name string `json:"name,omitempty"`

	// ToolCalls 是 assistant 请求调用的工具列表；每项包含调用 ID、类型和函数参数。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID 是 role 为 "tool" 的结果消息所对应的调用 ID，必须匹配先前 ToolCalls 中的 ID。
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ChatTool 声明模型可以调用的工具。目前 DeepSeek Chat Completions 支持 function 类型。
type ChatTool struct {
	// Type 工具类型，目前必须为 "function"。
	Type string `json:"type"`

	// Function 描述可供模型选择调用的函数。
	Function FunctionSpec `json:"function"`
}

// FunctionSpec 描述函数用途和参数 JSON Schema。它只提供调用说明，不会执行实际函数。
type FunctionSpec struct {
	// Name 是函数名称，必填；仅允许英文字母、数字、下划线和连字符，最长 128 个字符，且工具间必须唯一。
	Name string `json:"name"`

	// Description 是函数用途说明，帮助模型判断何时调用以及如何提供参数。
	Description string `json:"description,omitempty"`
	// Parameters 是描述函数参数的 JSON Schema 对象；省略时表示无参数。
	Parameters map[string]any `json:"parameters,omitempty"`
	// Strict 为 true 时启用 Beta strict 模式，要求函数参数符合 JSON Schema；默认 false。
	Strict bool `json:"strict,omitempty"`
}

// ToolCall 是 assistant 消息中的一次函数调用请求。它表示模型要求调用，不代表函数已经执行。
type ToolCall struct {
	// ID 是本次调用的唯一标识，回传工具结果时放入 ChatMessage.ToolCallID。
	ID string `json:"id"`
	// Type 当前为 "function"。
	Type string `json:"type"`
	// Function 包含函数名和模型生成的参数。
	Function FunctionCall `json:"function"`
}

// FunctionCall 包含模型生成的函数名称和 JSON 格式参数字符串。调用前应校验参数，不能默认信任模型输出。
type FunctionCall struct {
	// Name 是模型请求调用的函数名，应与已声明的 FunctionSpec.Name 匹配。
	Name string `json:"name"`
	// Arguments 是 JSON 编码的参数字符串；解析后仍应校验类型、必填字段和业务权限。
	Arguments string `json:"arguments"`
}

// NamedToolChoice 用于 tool_choice，强制模型调用指定函数。
type NamedToolChoice struct {
	// Type 必须为 "function"。
	Type string `json:"type"`
	// Function 指定需要调用的函数。
	Function NamedFunctionChoice `json:"function"`
}

// NamedFunctionChoice 指定要调用的函数名称；该名称必须已包含在请求的 tools 中。
type NamedFunctionChoice struct {
	Name string `json:"name"`
}
