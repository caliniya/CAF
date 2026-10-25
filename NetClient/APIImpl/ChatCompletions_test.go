package apiImpl

import "testing"

// 基础映射:system/user 消息、工具声明、MaxTokens;零值字段(Temperature)
// 和 Reasoning 不得出现。
func TestEncodeRequestFixture(t *testing.T) {
	p := ChatCompletionsProtocol{}

	got, err := p.EncodeRequest(ChatRequest{
		Model: "deepseek-chat",
		Messages: []ChatMessage{
			{Role: "system", Content: "你是 CAF 助手"},
			{Role: "user", Content: "你好"},
		},
		Tools: []Tool{{
			Name:        "read_file",
			Description: "读取文件",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		}},
		MaxTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `{"model":"deepseek-chat","messages":[` +
		`{"role":"system","content":"你是 CAF 助手"},` +
		`{"role":"user","content":"你好"}],` +
		`"max_tokens":1024,"stream":true,` +
		`"tools":[{"type":"function","function":{"name":"read_file","description":"读取文件",` +
		`"parameters":{"properties":{"path":{"type":"string"}},"type":"object"}}}]}`
	if got != want {
		t.Fatalf("unexpected encode:\n got: %s\nwant: %s", got, want)
	}
}

// 工具调用往返:assistant 的 ToolCalls 与 tool 结果消息(带 tool_call_id)。
func TestEncodeRequestToolRoundTrip(t *testing.T) {
	p := ChatCompletionsProtocol{}

	got, err := p.EncodeRequest(ChatRequest{
		Model: "deepseek-chat",
		Messages: []ChatMessage{
			{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:        "c1",
					Name:      "read_file",
					Arguments: `{"path":"a"}`,
				}},
			},
			{Role: "tool", ToolCallID: "c1", Content: "文件内容"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `{"model":"deepseek-chat","messages":[` +
		`{"role":"assistant","tool_calls":[{"id":"c1","type":"function",` +
		`"function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]},` +
		`{"role":"tool","content":"文件内容","tool_call_id":"c1"}],` +
		`"stream":true}`
	if got != want {
		t.Fatalf("unexpected encode:\n got: %s\nwant: %s", got, want)
	}
}
