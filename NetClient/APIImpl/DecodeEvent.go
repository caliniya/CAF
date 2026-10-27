package apiImpl

import (
	"encoding/json"
	"strings"
)

// ccStreamChunk 是 Chat Completions 流式响应的单个 JSON chunk。
type ccStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// ccToolCallDraft 是工具调用分片的聚合中间态:一次调用在流里被拆成多行,
// 首行带 index/id/name,后续行只追加 arguments 碎片。
type ccToolCallDraft struct {
	ID        string
	Name      string
	Arguments string
}

// DecodeEvent 把一行 SSE 数据载荷解码为内部增量。
//
// 状态:tool_calls 需要跨行聚合,所以聚合中间态放在实例字段上——
// 每条流必须 new 一个 ChatCompletionAPI,并行对话才不会互相污染。
func (api *ChatCompletionAPI) DecodeEvent(text string) ([]Delta, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil // 心跳/空行,跳过
	}
	if text == "[DONE]" {
		return []Delta{{Done: true}}, nil
	}

	var chunk ccStreamChunk
	if err := json.Unmarshal([]byte(text), &chunk); err != nil {
		return nil, err
	}
	if chunk.Error != nil {
		return nil, &StreamError{Message: chunk.Error.Message, Code: chunk.Error.Code}
	}

	var deltas []Delta
	finished := false

	for _, choice := range chunk.Choices {
		if choice.Delta.ReasoningContent != "" {
			deltas = append(deltas, Delta{Reasoning: choice.Delta.ReasoningContent})
		}
		if choice.Delta.Content != "" {
			deltas = append(deltas, Delta{Content: choice.Delta.Content})
		}
		for _, tc := range choice.Delta.ToolCalls {
			api.accumulateToolCall(tc.Index, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			finished = true
		}
	}

	// 流结束时把聚合完整的工具调用一次性吐出,上层拿到的永远是完整调用。
	if finished {
		if calls := api.takeToolCalls(); len(calls) > 0 {
			deltas = append(deltas, Delta{ToolCalls: calls})
		}
	}

	return deltas, nil
}

// accumulateToolCall 按 index 累积工具调用分片。
func (api *ChatCompletionAPI) accumulateToolCall(index int, id, name, args string) {
	if index < 0 {
		index = 0
	}
	for len(api.toolDrafts) <= index {
		api.toolDrafts = append(api.toolDrafts, ccToolCallDraft{})
	}
	draft := &api.toolDrafts[index]
	if id != "" {
		draft.ID = id
	}
	if name != "" {
		draft.Name = name
	}
	draft.Arguments += args
}

// takeToolCalls 取出并清空已聚合的工具调用。
func (api *ChatCompletionAPI) takeToolCalls() []ToolCall {
	if len(api.toolDrafts) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(api.toolDrafts))
	for _, d := range api.toolDrafts {
		if d.Name == "" {
			continue // 空占位,丢弃
		}
		calls = append(calls, ToolCall{ID: d.ID, Name: d.Name, Arguments: d.Arguments})
	}
	api.toolDrafts = nil
	return calls
}

// StreamError 表示服务端在流中返回的错误对象。
type StreamError struct {
	Message string
	Code    any
}

func (e *StreamError) Error() string {
	if e.Code != nil {
		return "模型服务错误: " + e.Message
	}
	return "模型服务错误: " + e.Message
}
