package tests

import (
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// 用真实 SSE 样本验证 DecodeEvent:开场 role chunk 不产生增量、
// 正文逐段累积、finish_reason 行无内容、[DONE] 产生 Done。
func TestDecodeEventRealSample(t *testing.T) {
	samples := []string{
		`{"choices":[{"delta":{"role":"assistant"},"finish_reason":null,"index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}`,
		`{"choices":[{"delta":{"content":"你好"},"finish_reason":null,"index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}`,
		`{"choices":[{"delta":{"content":"！"},"finish_reason":null,"index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}`,
		`{"choices":[{"delta":{"role":"assistant"},"finish_reason":"stop","index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk","usage":{"total_tokens":31}}`,
		`[DONE]`,
	}

	proto := &apiImpl.ChatCompletionAPI{}
	var content strings.Builder
	var done bool

	for _, s := range samples {
		deltas, err := proto.DecodeEvent(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		for _, d := range deltas {
			content.WriteString(d.Content)
			if d.Done {
				done = true
			}
		}
	}

	if content.String() != "你好！" {
		t.Fatalf("unexpected content: %q", content.String())
	}
	if !done {
		t.Fatal("[DONE] did not produce Done delta")
	}
}

// 思考增量走 Reasoning 字段,与正文分开。
func TestDecodeEventReasoning(t *testing.T) {
	proto := &apiImpl.ChatCompletionAPI{}
	deltas, err := proto.DecodeEvent(
		`{"choices":[{"delta":{"reasoning_content":"先想一想"},"finish_reason":null,"index":0}]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 1 || deltas[0].Reasoning != "先想一想" || deltas[0].Content != "" {
		t.Fatalf("unexpected deltas: %+v", deltas)
	}
}

func TestDecodeEventUsage(t *testing.T) {
	proto := &apiImpl.ChatCompletionAPI{}
	deltas, err := proto.DecodeEvent(
		`{"choices":[],"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 1 || deltas[0].Usage == nil {
		t.Fatalf("expected usage delta, got %+v", deltas)
	}
	usage := deltas[0].Usage
	if usage.PromptTokens != 120 || usage.CompletionTokens != 30 || usage.TotalTokens != 150 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

// 工具调用分片:首行带 index/id/name,后续行只追加 arguments,
// 在 finish_reason=tool_calls 时一次性吐出完整调用。
func TestDecodeEventToolCallAggregation(t *testing.T) {
	proto := &apiImpl.ChatCompletionAPI{}

	// 碎片行不产出任何增量。
	for _, s := range []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a\"}"}}]},"finish_reason":null}]}`,
	} {
		deltas, err := proto.DecodeEvent(s)
		if err != nil {
			t.Fatal(err)
		}
		if len(deltas) != 0 {
			t.Fatalf("fragment should not emit delta: %+v", deltas)
		}
	}

	// 结束行吐出完整调用。
	deltas, err := proto.DecodeEvent(
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 1 || len(deltas[0].ToolCalls) != 1 {
		t.Fatalf("expected one aggregated tool call, got %+v", deltas)
	}
	tc := deltas[0].ToolCalls[0]
	if tc.ID != "c1" || tc.Name != "read_file" || tc.Arguments != `{"path":"a"}` {
		t.Fatalf("unexpected tool call: %+v", tc)
	}
}

// 心跳/空载荷跳过且不报错;坏 JSON 才报错。
func TestDecodeEventIgnoresEmptyAndRejectsBadJSON(t *testing.T) {
	proto := &apiImpl.ChatCompletionAPI{}

	for _, s := range []string{"", "   "} {
		deltas, err := proto.DecodeEvent(s)
		if err != nil || len(deltas) != 0 {
			t.Fatalf("empty payload should be skipped: %v %+v", err, deltas)
		}
	}

	if _, err := proto.DecodeEvent(`{"choices":`); err == nil {
		t.Fatal("malformed json accepted")
	}
}
