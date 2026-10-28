package tests

// Chat Completions 协议编码测试:锁定内部结构 → 线格式的映射行为。

import (
	"testing"
	"time"

	"context"
	"net/http"
	"net/http/httptest"

	netclient "github.com/caliniya/caf/NetClient"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// 基础映射:system/user 消息、工具声明、MaxTokens;零值字段(Temperature)
// 和 Reasoning 不得出现。
func TestEncodeRequestFixture(t *testing.T) {
	p := &apiImpl.ChatCompletionAPI{}

	got, err := p.EncodeRequest(apiImpl.ChatRequest{
		Model: "deepseek-chat",
		Messages: []apiImpl.ChatMessage{
			{Role: "system", Content: "你是 CAF 助手"},
			{Role: "user", Content: "你好"},
		},
		Tools: []apiImpl.Tool{{
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
		`"max_tokens":1024,"stream":true,"stream_options":{"include_usage":true},` +
		`"tools":[{"type":"function","function":{"name":"read_file","description":"读取文件",` +
		`"parameters":{"properties":{"path":{"type":"string"}},"type":"object"}}}]}`
	if got != want {
		t.Fatalf("unexpected encode:\n got: %s\nwant: %s", got, want)
	}
}

func TestGetModelContextWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/model" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"deepseek-chat","name":"DeepSeek Chat","context_window":128000}`))
	}))
	defer server.Close()

	client := netclient.NewAPIClient(netclient.Options{ServerAddress: server.URL, APIKey: "sk-test"})
	got, err := client.GetModelContextWindow(context.Background(), "deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	if got != 128000 {
		t.Fatalf("unexpected context window: %d", got)
	}
}

func TestNewAPIClientAutomaticallyFetchesModels(t *testing.T) {
	requested := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model" {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		requested <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"deepseek-chat","name":"DeepSeek Chat","context_window":128000,"max_output_tokens":8192}}`))
	}))
	defer server.Close()

	client := netclient.NewAPIClient(netclient.Options{
		ServerAddress: server.URL,
		APIKey:        "sk-test",
		Model:         "deepseek-chat",
	})

	select {
	case <-requested:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not automatically request model metadata")
	}

	models, err := client.GetModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "DeepSeek Chat" || models[0].ContextWindow != 128000 {
		t.Fatalf("unexpected model info: %+v", models)
	}
}

// 工具调用往返:assistant 的 ToolCalls 与 tool 结果消息(带 tool_call_id);
// Reasoning 必须回传为 reasoning_content(DeepSeek 对携带 tools 的请求强制要求)。
func TestEncodeRequestToolRoundTrip(t *testing.T) {
	p := &apiImpl.ChatCompletionAPI{}

	got, err := p.EncodeRequest(apiImpl.ChatRequest{
		Model: "deepseek-chat",
		Messages: []apiImpl.ChatMessage{
			{
				Role:      "assistant",
				Content:   "我需要读一下文件",
				Reasoning: "用户让我处理文件,先调用工具",
				ToolCalls: []apiImpl.ToolCall{{
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
		`{"role":"assistant","content":"我需要读一下文件",` +
		`"tool_calls":[{"id":"c1","type":"function",` +
		`"function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}],` +
		`"reasoning_content":"用户让我处理文件,先调用工具"},` +
		`{"role":"tool","content":"文件内容","tool_call_id":"c1"}],` +
		`"stream":true,"stream_options":{"include_usage":true}}`
	if got != want {
		t.Fatalf("unexpected encode:\n got: %s\nwant: %s", got, want)
	}
}
