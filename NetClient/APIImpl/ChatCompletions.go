package apiImpl

import "encoding/json"

// ChatCompletionsProtocol 是 apiImpl.Protocol 的 Chat Completions 实现,
// 适用于 DeepSeek / SiliconFlow / OpenAI 兼容服务。
// 目前只实现 EncodeRequest;DecodeEvent(含 tool_calls 分片聚合、reasoning 增量)待续。
type ChatCompletionsProtocol struct{}

// EncodeRequest 把内部通用请求编码为 Chat Completions 请求体。
// 字段映射:Messages 逐条转换;Tools 统一为 function 类型;
// MaxTokens/Temperature 零值不发送(靠 omitempty);
// Thinking(内部 "off"/"low"/"medium"/"high")v1 暂不发送——各家支持差异大
// (DeepSeek 的 thinking/reasoning_effort、Qwen 的 enable_thinking),
// 后续按模型加开关时映射到 Thinking/ReasoningEffort 字段。
func (ChatCompletionsProtocol) EncodeRequest(req ChatRequest) (string, error) {
	body := ChatCompletionsRequest{
		Model:    req.Model,
		Stream:   true, // 客户端只走流式
		Messages: make([]CCChatMessage, 0, len(req.Messages)),
	}

	for _, m := range req.Messages {
		body.Messages = append(body.Messages, toCCMessage(m))
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

// toCCMessage 把一条内部消息转换为线格式消息。
// Reasoning 故意不映射:思考内容是模型输出,回传历史时不发送。
// Content 为 any 类型,其 omitempty 只识别 nil——空字符串要显式不装,
// 否则 assistant 的工具调用消息会带上多余的 "content":""。
func toCCMessage(m ChatMessage) CCChatMessage {
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
