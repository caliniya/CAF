package apiImpl

import "encoding/json"

// ChatCompletionAPI 是 apiImpl.Protocol 的 Chat Completions 实现,目前主要考虑deepseeek格式
type ChatCompletionAPI struct{}

func (api *ChatCompletionAPI) EncodeRequest(req ChatRequest) (string, error) {
	body := ChatCompletionsRequest{
		Model:           req.Model,
		Stream:          true, // 客户端只走流式
		Messages:        make([]CCChatMessage, 0, len(req.Messages)),
		ReasoningEffort: req.Thinking,
	}

	for _, m := range req.Messages {
		body.Messages = append(body.Messages, api.toCCMessage(m))
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, CCChatTool{
			Type: "function",
			Function: CCFunctionSpec{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		t := req.Temperature
		body.Temperature = &t
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// toCCMessage 把一条内部消息转换为对应格式
func (api *ChatCompletionAPI) toCCMessage(m ChatMessage) CCChatMessage {
	cc := CCChatMessage{Role: m.Role}
	if m.Content != "" {
		cc.Content = m.Content
	}

	switch m.Role {
	case "tool":
		cc.ToolCallID = m.ToolCallID
	case "assistant":
		for _, tc := range m.ToolCalls {
			cc.ToolCalls = append(cc.ToolCalls, CCToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: CCFunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
	}

	return cc
}

// ChatCompletionsRequest 表示 DeepSeek Chat Completions API 的 JSON 请求体。
// 内部结构体简写为CC
type ChatCompletionsRequest struct {
	// Model 是模型 ID，必填；可用模型以账号可调用列表和文档为准。
	Model string `json:"model"`

	// Messages 是对话消息列表，必填且至少包含一条消息；按顺序传入 system、user、ksassistant 等消息。
	Messages []CCChatMessage `json:"messages"`

	// Thinking 控制思考模式；Type 可设为 "enabled" 或 "disabled"。省略时由服务端决定默认行为。
	Thinking *CCThinkingConfig `json:"thinking,omitempty"`

	// ReasoningEffort 控制思考开关或强度，可设为 "none"、"low"、"high"、"max"；
	// 文档兼容值 "minimal" 映射为 "low"，"medium" / "xhigh" 映射为 "high"。
	// 它与 Thinking 都用于控制思考行为，具体支持组合以当前模型文档为准。
	ReasoningEffort string `json:"reasoning_effort,omitempty"`

	// MaxTokens 限制生成 token 数，DeepSeek 文档范围为 1 到 393216；省略时使用模型默认值。
	MaxTokens int `json:"max_tokens,omitempty"`

	// ResponseFormat 指定输出格式；Type 可设为 "text" 或 "json_object"。
	// 使用 JSON 模式时，还应在消息中明确要求模型输出 JSON。
	ResponseFormat *CCResponseFormat `json:"response_format,omitempty"`

	// Stop 是停止序列，可传一个字符串或最多包含 16 个字符串的切片。
	Stop any `json:"stop,omitempty"`

	// Stream 控制是否以 SSE 流式返回；流结束标记为 data: [DONE]。省略或 false 表示非流式。
	Stream bool `json:"stream,omitempty"`

	// StreamOptions 配置流式返回选项；只有 Stream 为 true 时才能设置。
	StreamOptions *CCStreamOptions `json:"stream_options,omitempty"`

	// Temperature 控制采样随机性，范围为 0 到 2；思考模式下不生效。
	// 使用指针以支持显式发送 0；nil 表示省略字段。
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP 控制 nucleus sampling，范围为 (0, 1]；思考模式下仅 0.95 到 1.0 有效，
	// 低于 0.95 会按 0.95 处理，非思考模式下该参数会被忽略。
	TopP *float64 `json:"top_p,omitempty"`

	// Tools 声明模型可调用的工具；目前支持 function 类型。模型只返回调用请求，函数需由客户端执行。
	Tools []CCChatTool `json:"tools,omitempty"`

	// ToolChoice 控制工具调用策略，可传 "none"、"auto"、"required"，或 NamedToolChoice 指定函数。
	// 有 tools 时默认 auto，无 tools 时默认 none；思考模式下不支持 required 或指定函数。
	ToolChoice any `json:"tool_choice,omitempty"`

	// Logprobs 为 true 时在响应中返回输出 token 的对数概率。
	Logprobs bool `json:"logprobs,omitempty"`

	// TopLogprobs 指定每个输出位置返回概率最高的 token 数量，范围为 0 到 20；需要同时启用 Logprobs。
	TopLogprobs *int `json:"top_logprobs,omitempty"`

	// UserID 是可选的业务侧用户标识，最多 512 个字符，仅允许字母、数字、连字符和下划线；不要放隐私信息。
	UserID string `json:"user_id,omitempty"`
}

// ThinkingConfig 控制是否启用模型思考模式。
type CCThinkingConfig struct {
	// Type 为 "enabled" 启用思考模式，为 "disabled" 关闭思考模式。
	Type string `json:"type"`
}

// ResponseFormat 指定模型的响应格式。
type CCResponseFormat struct {
	// Type 可为 "text"（普通文本）或 "json_object"（JSON 模式）。
	Type string `json:"type"`
}

// StreamOptions 配置流式响应的附加信息。
type CCStreamOptions struct {
	// IncludeUsage 为 true 时在流式响应中包含 token 用量统计；必须同时启用 Stream。
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatMessage 表示对话中的一条消息，也可承载模型的工具调用或工具执行结果。
type CCChatMessage struct {
	// Role 表示消息角色，常见可用值为 "system"、"user"、"assistant"、"tool"。
	// tool 角色用于回传工具执行结果；assistant 消息可包含 ToolCalls。
	Role string `json:"role"`

	// Content 是消息文本；普通文本对话传 string。工具调用的 assistant 消息中可省略或为 null。
	Content any `json:"content,omitempty"`

	// Name 可选，用于标识消息参与者或工具名称；普通 user/assistant 消息通常省略。
	Name string `json:"name,omitempty"`

	// ToolCalls 是 assistant 请求调用的工具列表；每项包含调用 ID、类型和函数参数。
	ToolCalls []CCToolCall `json:"tool_calls,omitempty"`

	// ToolCallID 是 role 为 "tool" 的结果消息所对应的调用 ID，必须匹配先前 ToolCalls 中的 ID。
	ToolCallID string `json:"tool_call_id,omitempty"`

	// 保留模型返回的思考内容，供工具调用后续请求回传。
	ReasoningContent *string `json:"reasoning_content,omitempty"`
}

// ChatTool 声明模型可以调用的工具。目前 DeepSeek Chat Completions 支持 function 类型。
type CCChatTool struct {
	// Type 工具类型，目前必须为 "function"。
	Type string `json:"type"`

	// Function 描述可供模型选择调用的函数。
	Function CCFunctionSpec `json:"function"`
}

// FunctionSpec 描述函数用途和参数 JSON Schema。它只提供调用说明，不会执行实际函数。
type CCFunctionSpec struct {
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
type CCToolCall struct {
	// ID 是本次调用的唯一标识，回传工具结果时放入 ChatMessage.ToolCallID。
	ID string `json:"id"`
	// Type 当前为 "function"。
	Type string `json:"type"`
	// Function 包含函数名和模型生成的参数。
	Function CCFunctionCall `json:"function"`
}

// FunctionCall 包含模型生成的函数名称和 JSON 格式参数字符串。调用前应校验参数，不能默认信任模型输出。
type CCFunctionCall struct {
	// Name 是模型请求调用的函数名，应与已声明的 FunctionSpec.Name 匹配。
	Name string `json:"name"`
	// Arguments 是 JSON 编码的参数字符串；解析后仍应校验类型、必填字段和业务权限。
	Arguments string `json:"arguments"`
}

// NamedToolChoice 用于 tool_choice，强制模型调用指定函数。
type CCNamedToolChoice struct {
	// Type 必须为 "function"。
	Type string `json:"type"`
	// Function 指定需要调用的函数。
	Function CCNamedFunctionChoice `json:"function"`
}

// NamedFunctionChoice 指定要调用的函数名称；该名称必须已包含在请求的 tools 中。
type CCNamedFunctionChoice struct {
	Name string `json:"name"`
}
