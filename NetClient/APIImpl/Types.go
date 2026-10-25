package apiImpl

// 本文件定义全项目通用的对话内部结构：协议无关、传输无关。
// 协议实现负责把它编码成各家线格式；core/agent/前端只认这里的一套。

// ChatMessage 是一条对话消息。
type ChatMessage struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string

	// 思考内容：支持思考的模型（DeepSeek-R1、Qwen thinking 等）会分两路输出，
	// Reasoning 与 Content 分开携带，前端分开展示。
	Reasoning string

	// assistant 发起的工具调用；一条消息可能同时携带 Content 和多个 ToolCalls。
	ToolCalls []ToolCall

	// Role == "tool" 时有效：本次结果对应哪一次调用（与 ToolCall.ID 对齐）。
	ToolCallID string
}

// ToolCall 是一次工具调用。Arguments 是 JSON 对象的字符串形式
// （OpenAI 惯例：流式分片聚合后交给上层再解析）。
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Tool 是提供给模型的工具声明：Name/Description/Parameters(JSON Schema)
// 原样进入请求体，由模型决定是否调用。
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ChatRequest 是一次对话请求（协议无关）。
// Thinking 取 "off"|"low"|"medium"|"high"，由协议实现映射成自家字段；
// MaxTokens/Temperature 为零值时表示不发送该字段。
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Tools       []Tool
	Thinking    string
	MaxTokens   int
	Temperature float64
}

type Delta struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCall
	Done      bool
}
