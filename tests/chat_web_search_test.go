package Tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	chat "github.com/caliniya/caf/core/Chat"
)

func TestWebSearchWireUsesOnlyTheSelectedProvider(t *testing.T) {
	for _, tc := range []struct {
		mode   api.NativeSearchMode
		skip   bool
		fields string
	}{
		{api.SearchDeepSeek, false, `{"tools":[{"type":"deepseek_web_search"}]}`}, {api.SearchDeepSeek, true, `{}`},
		{api.SearchDashScope, false, `{"enable_search":true}`}, {api.SearchDashScope, true, `{"enable_search":false}`},
		{api.SearchSonar, false, `{"disable_search":false,"enable_search_classifier":true}`}, {api.SearchSonar, true, `{"disable_search":true}`},
		{api.SearchAlwaysOn, false, `{}`}, {api.SearchAlwaysOn, true, `{}`}, {"", false, `{}`},
	} {
		t.Run(fmt.Sprintf("%s/skip=%t", tc.mode, tc.skip), func(t *testing.T) {
			body, err := (&api.ChatCompletionAPI{}).EncodeRequest(api.ChatRequest{Model: "m", Messages: []api.ChatMessage{{Role: "user", Content: "hello"}}, SkipWebSearch: tc.skip, SearchMode: tc.mode})
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

func TestChatWebSearchAutomaticallyFollowsModelAndProvider(t *testing.T) {
	c, _ := approvalCore(t)
	cfg := c.Configs.EnablesConfig
	cfg.ServerAddress = "https://api.deepseek.com/v1/chat/completions"
	cfg.EnabledModel = "deepseek-flash"
	cfg.Model = []api.Model{{ID: cfg.EnabledModel}, {ID: "deepseek-v4-pro"}}
	a, err := c.CreateChat("", "", "", "")
	checkOK(t, err)
	var calls int
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = searchTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 5 {
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
		if local == 0 || search != map[int]int{1: 1, 2: 1, 3: 1}[calls] {
			t.Errorf("request %d: local=%d, native=%d", calls, local, search)
		}
		replay, searchInstructions := 0, 0
		for _, msg := range req.Messages {
			if body, ok := msg.Content.(string); ok && msg.Role == "system" && strings.Contains(body, "deepseek_web_search") {
				searchInstructions++
			}
			if len(msg.ServerToolCalls) != 0 {
				replay++
				if !strings.Contains(string(msg.ServerToolCalls), `"opaque-step"`) {
					t.Error("lost signed step")
				}
			}
		}
		if searchInstructions != search {
			t.Errorf("request %d: advertised search %d times, offered %d native tools", calls, searchInstructions, search)
		}
		if (calls == 3) != (replay == 1) || replay > 1 {
			t.Errorf("request %d replays %d server steps", calls, replay)
		}
		var chunk = `{"choices":[{"delta":{"content":"hello"},"finish_reason":"stop"}]}`
		if calls == 2 {
			chunk = `{"choices":[{"delta":{"content":"answer","server_tool_calls":[{"index":0,"calls":[{"type":"deepseek_web_search","status":"completed","signature":"opaque-step","sources":[{"url":"https://example.org/one","title":"Source"}]}]}],"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.org/one"}}]},"finish_reason":"stop"}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n"))}, nil
	})
	checkOK(t, c.SendChatMessage(a.ID, "hello", "none"))
	// A conversation saved with the old switch off must behave like a new chat.
	a, err = c.GetChat(a.ID)
	checkOK(t, err)
	legacy, err := json.Marshal(struct {
		*chat.ChatTask
		Enabled bool `json:"webSearchEnabled"`
	}{ChatTask: a, Enabled: false})
	checkOK(t, err)
	checkOK(t, os.WriteFile(filepath.Join(c.Paths.Data, "chats", a.ID+".json"), legacy, 0o600))
	checkOK(t, c.SendChatMessage(a.ID, "search", "high"))
	checkOK(t, c.SendChatMessage(a.ID, "follow up", "high"))
	a, err = c.GetChat(a.ID)
	checkOK(t, err)
	path, err := a.Tree.CurrentPath()
	checkOK(t, err)
	if len(path) != 6 || path[1].Child[0].WebSearch != nil || path[5].Child[0].WebSearch != nil {
		t.Fatal("ordinary replies acquired a search receipt")
	}
	if !path[1].WebSearchOffered || !path[3].WebSearchOffered || !path[5].WebSearchOffered {
		t.Fatal("new and legacy chats must offer native search even when no search is performed")
	}
	searched := path[3].Child[0]
	if len(searched.ToolCalls) != 0 || searched.WebSearch == nil || len(searched.WebSearch.Sources) != 1 || searched.WebSearch.Sources[0].Title != "Source" {
		t.Fatalf("native search executed locally or missing sources: %+v", searched)
	}
	_, err = c.UpdateChatModel(a.ID, cfg.ID, "deepseek-v4-pro")
	checkOK(t, err)
	checkOK(t, c.SendChatMessage(a.ID, "same provider, different model", "high"))
	_, err = c.UpdateChatModel(a.ID, cfg.ID, "deepseek-flash")
	checkOK(t, err)
	// Even a proxy reporting native_web_search=true cannot inherit DeepSeek's wire protocol or opaque history.
	cfg.ServerAddress = "https://compatible.example/v1/chat/completions"
	cfg.Model[0].NativeWebSearch = new(true)
	cfg.Model[0].NativeSearchMode = api.SearchDeepSeek
	checkOK(t, c.SendChatMessage(a.ID, "same ID, different provider", "high"))
	a, err = c.GetChat(a.ID)
	checkOK(t, err)
	path, err = a.Tree.CurrentPath()
	checkOK(t, err)
	if len(path) != 10 || path[7].WebSearchOffered || path[9].WebSearchOffered || path[3].Child[0].WebSearch == nil {
		t.Fatal("changing models or routes must not offer unsupported search or erase earlier receipts")
	}
	if calls != 5 {
		t.Fatalf("expected five user-initiated requests, got %d", calls)
	}
}

func TestQQAutomaticWebSearchRespectsTriggerAndSkipsJudgments(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	f := useQQChat(t, map[string]string{"proactiveEnabled": "true", "proactiveCooldown": "0"}, nil)
	configs, err := f.core.AddModelConfig("search", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", "chat-completions", "", []api.Model{{ID: "qwen-plus"}})
	checkOK(t, err)
	settings, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
	checkOK(t, err)
	settings.ConfigID, settings.Model = configs.EnablesConfig.ID, "qwen-plus"
	_, err = f.core.UpdateClientExtensionChatConfig(f.clientID, f.extensionID, *settings)
	checkOK(t, err)
	http.DefaultTransport = searchTestTransport(func(r *http.Request) (*http.Response, error) {
		var req api.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		if r.URL.Host != "dashscope.aliyuncs.com" || req.Model != settings.Model {
			return nil, fmt.Errorf("unexpected model route: %s, %s", r.URL.Host, req.Model)
		}
		f.requests <- req
		response := httptest.NewRecorder()
		qqSSE(response, "false")
		return response.Result(), nil
	})
	for _, mention := range []bool{true, false} {
		req := f.exchange(t, "group", 30003, 23456, "助手 查一下", mention)
		if req.EnableSearch == nil || *req.EnableSearch != mention || strings.Contains(qqRequestText(req), "本轮已启用服务商的原生联网搜索") != mention {
			t.Fatalf("QQ search and its instructions must follow this turn's tool scope (mention=%t)", mention)
		}
	}
	f.message(t, "group", 30003, 23456, "ambient", false)
	decision := f.request(t)
	if len(decision.Tools) != 0 || decision.EnableSearch == nil || *decision.EnableSearch || !strings.Contains(qqRequestText(decision), "判断是否有必要主动参与") {
		t.Fatal("proactive reply decision unexpectedly enabled search")
	}
}

func TestWebSearchEvidenceRequiresStructuredProviderData(t *testing.T) {
	const source = "https://example.org/news"
	for _, tc := range []struct {
		mode    api.NativeSearchMode
		skip    bool
		chunk   string
		used    bool
		sources int
	}{
		{api.SearchSonar, false, `{"citations":["https://example.org/news","javascript:void(0)"],"search_results":[{"url":"https://example.org/news","title":"News"}]}`, true, 1},
		{api.SearchAlwaysOn, true, `{"choices":[{"delta":{"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.org/news"}}]}}]}`, true, 1},
		{api.SearchDashScope, false, `{"search_info":{"search_results":[{"url":"https://example.org/news"}]}}`, true, 1},
		{api.SearchDeepSeek, false, `{"choices":[{"delta":{"server_tool_calls":[{"calls":[{"type":"deepseek_web_search","status":"completed"}]}]}}]}`, true, 0},
		{api.SearchDeepSeek, false, `{"choices":[{"delta":{"server_tool_calls":[{"calls":[{"type":"deepseek_web_search","status":"failed"}]}]}}]}`, false, 0},
		{api.SearchSonar, false, `{"choices":[{"delta":{"content":"已联网搜索 https://example.org/news"}}]}`, false, 0},
		{api.SearchDeepSeek, true, `{"choices":[{"delta":{"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.org/news"}}]}}]}`, false, 0},
	} {
		p := &api.ChatCompletionAPI{}
		_, err := p.EncodeRequest(api.ChatRequest{SearchMode: tc.mode, SkipWebSearch: tc.skip})
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
			s, err = c.GetChat(s.ID)
			checkOK(t, err)
			path, err := s.Tree.CurrentPath()
			checkOK(t, err)
			if path[len(path)-1].WebSearchOffered != conflict {
				t.Fatalf("search availability should be recorded only after a successful request (retry=%t)", conflict)
			}
		})
	}
}

func TestDeepSeekSearchHistoryIsBoundToProviderConfig(t *testing.T) {
	m := api.ChatMessage{Role: "assistant", Content: "answer", SearchConfigID: "owner-a", ServerToolCalls: json.RawMessage(`[{"index":0,"calls":[]}]`)}
	for _, tc := range []struct {
		id     string
		skip   bool
		replay bool
	}{{"owner-a", false, true}, {"owner-a", true, false}, {"owner-b", false, false}} {
		wire, err := (&api.ChatCompletionAPI{}).EncodeRequest(api.ChatRequest{SearchMode: api.SearchDeepSeek, SkipWebSearch: tc.skip, SearchConfigID: tc.id, Messages: []api.ChatMessage{m}})
		checkOK(t, err)
		if strings.Contains(wire, "server_tool_calls") != tc.replay || strings.Contains(wire, "owner-") {
			t.Errorf("unscoped search context: %s", wire)
		}
	}
}
