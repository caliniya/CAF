package Tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	api "github.com/caliniya/caf/NetClient/APIImpl"
)

func TestWebSearchWireUsesOnlyTheSelectedProvider(t *testing.T) {
	for _, tc := range []struct {
		mode   api.NativeSearchMode
		on     bool
		fields string
	}{
		{api.SearchDeepSeek, true, `{"tools":[{"type":"deepseek_web_search"}]}`}, {api.SearchDeepSeek, false, `{}`},
		{api.SearchDashScope, true, `{"enable_search":true}`}, {api.SearchDashScope, false, `{"enable_search":false}`},
		{api.SearchSonar, true, `{"disable_search":false,"enable_search_classifier":true}`}, {api.SearchSonar, false, `{"disable_search":true}`},
		{api.SearchAlwaysOn, false, `{}`}, {"", true, `{}`},
	} {
		t.Run(fmt.Sprintf("%s/%t", tc.mode, tc.on), func(t *testing.T) {
			body, err := (&api.ChatCompletionAPI{}).EncodeRequest(api.ChatRequest{Model: "m", Messages: []api.ChatMessage{{Role: "user", Content: "hello"}}, WebSearch: tc.on, SearchMode: tc.mode})
			checkOK(t, err)
			var got, want map[string]any
			checkOK(t, json.Unmarshal([]byte(body), &got))
			checkOK(t, json.Unmarshal([]byte(tc.fields), &want))
			for _, key := range []string{"model", "messages", "stream", "stream_options"} {
				delete(got, key)
			}
			g, _ := json.Marshal(got)
			w, _ := json.Marshal(want)
			if string(g) != string(w) {
				t.Fatalf("got %s; want %s", g, w)
			}
		})
	}
}

type searchTestTransport func(*http.Request) (*http.Response, error)

func (f searchTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestChatWebSearchPreferenceAndProviderReceipts(t *testing.T) {
	c, _ := approvalCore(t)
	cfg := c.Configs.EnablesConfig
	cfg.ServerAddress = "https://api.deepseek.com/v1/chat/completions"
	cfg.EnabledModel = "deepseek-flash"
	cfg.Model = []api.Model{{ID: cfg.EnabledModel}}
	a, err := c.CreateChat("", "", "", "")
	checkOK(t, err)
	b, err := c.CreateChat("", "", "", "")
	checkOK(t, err)
	var calls int
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = searchTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 4 {
			return nil, fmt.Errorf("unexpected extra request %d", calls)
		}
		var req api.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		var local, search int
		for _, tool := range req.Tools {
			switch tool.Type {
			case "deepseek_web_search":
				search++
				if tool.Function != nil {
					t.Error("search encoded as local function")
				}
			case "function":
				local++
			}
		}
		if local == 0 || search != map[int]int{2: 1, 3: 1}[calls] {
			t.Errorf("request %d: local=%d, native=%d", calls, local, search)
		}
		replay := 0
		for _, msg := range req.Messages {
			if len(msg.ServerToolCalls) != 0 {
				replay++
				if !strings.Contains(string(msg.ServerToolCalls), `"opaque-step"`) {
					t.Error("lost signed step")
				}
			}
		}
		if (calls == 3) != (replay == 1) || replay > 1 {
			t.Errorf("request %d replays %d server steps", calls, replay)
		}
		var chunk = `{"choices":[{"delta":{"content":"hello"},"finish_reason":"stop"}]}`
		if calls == 2 {
			if _, err := c.SetChatWebSearch(a.ID, false); err == nil {
				t.Error("changed search setting during a generation")
			}
			chunk = `{"choices":[{"delta":{"content":"answer","server_tool_calls":[{"index":0,"calls":[{"type":"deepseek_web_search","status":"completed","signature":"opaque-step","sources":[{"url":"https://example.org/one","title":"Source"}]}]}],"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.org/one"}}]},"finish_reason":"stop"}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n"))}, nil
	})
	checkOK(t, c.SendChatMessage(a.ID, "hello", "none"))
	_, err = c.SetChatWebSearch(a.ID, true)
	checkOK(t, err)
	a, err = c.GetChat(a.ID)
	checkOK(t, err)
	b, err = c.GetChat(b.ID)
	checkOK(t, err)
	if !a.WebSearchEnabled || b.WebSearchEnabled {
		t.Fatal("search permission is not per chat or was not persisted")
	}
	checkOK(t, c.SendChatMessage(a.ID, "search", "high"))
	checkOK(t, c.SendChatMessage(a.ID, "follow up", "high"))
	a, err = c.GetChat(a.ID)
	checkOK(t, err)
	path, err := a.Tree.CurrentPath()
	checkOK(t, err)
	if len(path) != 6 || path[1].Child[0].WebSearch != nil || path[5].Child[0].WebSearch != nil {
		t.Fatal("ordinary replies acquired a search receipt")
	}
	searched := path[3].Child[0]
	if len(searched.ToolCalls) != 0 || searched.WebSearch == nil || len(searched.WebSearch.Sources) != 1 || searched.WebSearch.Sources[0].Title != "Source" {
		t.Fatalf("native search executed locally or missing sources: %+v", searched)
	}
	// Even a proxy reporting native_web_search=true cannot inherit DeepSeek's wire protocol or opaque history.
	cfg.ServerAddress = "https://compatible.example/v1/chat/completions"
	cfg.Model[0].NativeWebSearch = new(true)
	cfg.Model[0].NativeSearchMode = api.SearchDeepSeek
	checkOK(t, c.SendChatMessage(a.ID, "same ID, different provider", "high"))
	if calls != 4 {
		t.Fatalf("expected four user-initiated requests, got %d", calls)
	}
}

func TestWebSearchEvidenceRequiresStructuredProviderData(t *testing.T) {
	const source = "https://example.org/news"
	for _, tc := range []struct {
		mode    api.NativeSearchMode
		on      bool
		chunk   string
		used    bool
		sources int
	}{
		{api.SearchSonar, true, `{"citations":["https://example.org/news","javascript:void(0)"],"search_results":[{"url":"https://example.org/news","title":"News"}]}`, true, 1},
		{api.SearchAlwaysOn, false, `{"choices":[{"delta":{"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.org/news"}}]}}]}`, true, 1},
		{api.SearchDashScope, true, `{"search_info":{"search_results":[{"url":"https://example.org/news"}]}}`, true, 1},
		{api.SearchDeepSeek, true, `{"choices":[{"delta":{"server_tool_calls":[{"calls":[{"type":"deepseek_web_search","status":"completed"}]}]}}]}`, true, 0},
		{api.SearchDeepSeek, true, `{"choices":[{"delta":{"server_tool_calls":[{"calls":[{"type":"deepseek_web_search","status":"failed"}]}]}}]}`, false, 0},
		{api.SearchSonar, true, `{"choices":[{"delta":{"content":"已联网搜索 https://example.org/news"}}]}`, false, 0},
		{api.SearchDeepSeek, false, `{"choices":[{"delta":{"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.org/news"}}]}}]}`, false, 0},
	} {
		p := &api.ChatCompletionAPI{}
		_, err := p.EncodeRequest(api.ChatRequest{SearchMode: tc.mode, WebSearch: tc.on})
		checkOK(t, err)
		d, err := p.DecodeEvent(tc.chunk)
		checkOK(t, err)
		var receipt *api.SearchReceipt
		for _, part := range d {
			receipt = receipt.Merge(part.WebSearch)
		}
		if (receipt != nil) != tc.used || receipt != nil && (len(receipt.Sources) != tc.sources || tc.sources > 0 && receipt.Sources[0].URL != source) {
			t.Errorf("%s %s: unexpected receipt %+v", tc.mode, tc.chunk, receipt)
		}
	}
}

func TestNativeSearchRejectingMixedTools(t *testing.T) {
	for _, conflict := range []bool{true, false} {
		t.Run(fmt.Sprintf("conflict=%t", conflict), func(t *testing.T) {
			c, _ := approvalCore(t)
			cfg := c.Configs.EnablesConfig
			cfg.ServerAddress = "https://api.deepseek.com/chat/completions"
			cfg.EnabledModel = "deepseek-flash"
			cfg.Model = []api.Model{{ID: cfg.EnabledModel}}
			s, err := c.CreateChat("", "", "", "")
			checkOK(t, err)
			_, err = c.SetChatWebSearch(s.ID, true)
			checkOK(t, err)
			previous := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = previous })
			calls := 0
			http.DefaultTransport = searchTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var req api.ChatCompletionsRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					return nil, err
				}
				if calls == 1 {
					message := "invalid model"
					if conflict {
						message = "tools: other tools cannot be used together with deepseek_web_search"
					}
					return &http.Response{StatusCode: 400, Status: "400 Bad Request", Body: io.NopCloser(strings.NewReader(message))}, nil
				}
				if !conflict || calls > 3 || len(req.Tools) != 1 || req.Tools[0].Type != "deepseek_web_search" {
					return nil, fmt.Errorf("unexpected retry: %d, %+v", calls, req.Tools)
				}
				var chunk string
				if calls == 2 {
					chunk = `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"not-enabled","type":"function","function":{"name":"ReadFile","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
				} else {
					last := req.Messages[len(req.Messages)-1]
					if last.Role != "tool" || last.ToolCallID != "not-enabled" || last.Content != "此工具被禁用" {
						t.Errorf("unexpected tool execution after fallback: %+v", last)
					}
					chunk = `{"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n"))}, nil
			})
			err = c.SendChatMessage(s.ID, "search", "none")
			if conflict {
				checkOK(t, err)
				if calls != 3 {
					t.Fatal(calls)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("unrelated error retried or swallowed: %d, %v", calls, err)
			}
		})
	}
}

func TestDeepSeekSearchHistoryIsBoundToProviderConfig(t *testing.T) {
	m := api.ChatMessage{Role: "assistant", Content: "answer", SearchConfigID: "owner-a", ServerToolCalls: json.RawMessage(`[{"index":0,"calls":[]}]`)}
	for _, tc := range []struct {
		id     string
		on     bool
		replay bool
	}{{"owner-a", true, true}, {"owner-a", false, false}, {"owner-b", true, false}} {
		wire, err := (&api.ChatCompletionAPI{}).EncodeRequest(api.ChatRequest{SearchMode: api.SearchDeepSeek, WebSearch: tc.on, SearchConfigID: tc.id, Messages: []api.ChatMessage{m}})
		checkOK(t, err)
		if strings.Contains(wire, "server_tool_calls") != tc.replay || strings.Contains(wire, "owner-") {
			t.Errorf("unscoped search context: %s", wire)
		}
	}
}
