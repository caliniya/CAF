package Tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	persona "github.com/caliniya/caf/core/Persona"
)

func (f *qqChatFixture) tokenStats(t *testing.T) backend.QQTokenStats {
	t.Helper()
	options, err := f.core.ListExtensions()
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.ID == f.extensionID && option.Usage != nil {
			return *option.Usage
		}
	}
	t.Fatal("QQ extension usage is missing")
	return backend.QQTokenStats{}
}

func (f *qqChatFixture) waitTokenStats(t *testing.T, requests int64) backend.QQTokenStats {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		stats := f.tokenStats(t)
		if stats.Requests == requests {
			return stats
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d model requests, got %+v", requests, stats)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *qqChatFixture) restartQQ(t *testing.T) {
	t.Helper()
	q, _ := f.core.Servers.Get[*backend.QQServer](backend.QQServerName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetClientExtensionEnabled(f.clientID, f.extensionID, true); err != nil {
		t.Fatal(err)
	}
	adapter, err := f.core.GetQQAdapter(f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	f.connection = connectReverseWS(t, adapter.Address, "10001", "test-token")
}

func TestQQChatUsageIncludesJudgmentsAndEveryToolRound(t *testing.T) {
	var judgments, rounds atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false", "proactiveEnabled": "true", "proactiveCooldown": "0"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if strings.Contains(qqRequestText(request), "判断是否有必要主动参与") {
			if judgments.Add(1) == 1 {
				qqSSEUsage(w, `{"reply":false}`, &api.TokenUsage{PromptTokens: 1000, CompletionTokens: 10, TotalTokens: 1010,
					PromptCacheHitTokens: intPointer(500), PromptCacheMissTokens: intPointer(500)}, 2)
			} else {
				qqSSEUsage(w, `{"reply":true}`, &api.TokenUsage{PromptTokens: 200, CompletionTokens: 10, TotalTokens: 210,
					PromptTokensDetails: &api.PromptTokenDetails{CachedTokens: intPointer(0)}}, 1)
			}
			return
		}
		switch rounds.Add(1) {
		case 1:
			qqSSEUsage(w, "", &api.TokenUsage{PromptTokens: 3000, CompletionTokens: 100, TotalTokens: 3100,
				PromptCacheHitTokens: intPointer(2400), PromptCacheMissTokens: intPointer(600)}, 2,
				approvalCall("usage-write", "WriteFile", map[string]any{"path": []string{"usage.txt"}, "content": "written"}))
		case 2:
			qqSSEUsage(w, "完成", &api.TokenUsage{PromptTokens: 4000, CompletionTokens: 150, TotalTokens: 4150,
				PromptTokensDetails:     &api.PromptTokenDetails{CachedTokens: intPointer(1000)},
				CompletionTokensDetails: &api.CompletionTokenDetails{ReasoningTokens: intPointer(50)}}, 2)
		case 3:
			qqSSEUsage(w, "完成", &api.TokenUsage{PromptTokens: 150, CompletionTokens: 20}, 1)
		default:
			qqSSE(w, "完成")
		}
	})
	f.message(t, "group", 30003, 23456, "negative-judgment", false)
	f.request(t)
	f.waitTokenStats(t, 1)
	if len(f.contexts(t)) != 0 {
		t.Fatal("negative judgment created a context")
	}
	f.message(t, "group", 30003, 23456, "positive-judgment", false)
	for range 3 {
		f.request(t)
	}
	f.reply(t)
	stats := f.waitTokenStats(t, 4)
	expected := backend.QQTokenStats{Requests: 4, UsageRequests: 4, PromptTokens: 8200, CompletionTokens: 270, TotalTokens: 8470, CachedPromptTokens: 3900, CachePromptTokens: 8200}
	if stats != expected {
		t.Fatalf("judgments, tool rounds or repeated snapshots counted incorrectly: %+v", stats)
	}
	record := f.contexts(t)[0]
	if record.ContextTokens == nil || *record.ContextTokens != 4150 {
		t.Fatalf("context length accumulated lifetime input or reasoning twice: %+v", record)
	}
	if _, err := os.Stat(filepath.Join(record.Workspace.Path, "usage.txt")); !os.IsNotExist(err) {
		t.Fatalf("ordinary proactive reply executed a non-memory tool: %v", err)
	}
	clientOptions, err := f.core.ListClientExtensions(f.clientID)
	if err != nil || len(clientOptions) != 1 || clientOptions[0].Usage == nil || *clientOptions[0].Usage != stats {
		t.Fatalf("client extension listing omitted usage: %+v / %v", clientOptions, err)
	}
	f.exchange(t, "private", 30003, 0, "cache-unreported", false)
	stats = f.waitTokenStats(t, 5)
	if stats.TotalTokens != 8640 || stats.CachePromptTokens != 8200 || stats.CachedPromptTokens != 3900 {
		t.Fatalf("unreported cache became zero hits or missing total was not derived: %+v", stats)
	}
	f.exchange(t, "private", 30003, 0, "usage-unreported", false)
	stats = f.waitTokenStats(t, 6)
	if stats.UsageRequests != 5 || stats.TotalTokens != 8640 || f.contexts(t)[0].ContextTokens != nil {
		t.Fatalf("missing usage was presented as known: %+v / %+v", stats, f.contexts(t))
	}
}

func TestQQChatLongContextWarningAndUsageSurviveRestartClearDelete(t *testing.T) {
	var length atomic.Int64
	length.Store(299999)
	f := useQQChat(t, map[string]string{"groupContext": "false", "agentEnabled": "true", "proactiveEnabled": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		tokens := int(length.Load())
		qqSSEUsage(w, "完成", &api.TokenUsage{PromptTokens: tokens - 20, CompletionTokens: 20, TotalTokens: tokens,
			PromptTokensDetails: &api.PromptTokenDetails{CachedTokens: intPointer(0)}}, 1)
	})
	f.exchange(t, "private", 30003, 0, "below-threshold", false)
	if item := f.contexts(t)[0]; item.ContextTokens == nil || *item.ContextTokens != 299999 || item.ContextWarningSent {
		t.Fatalf("warning fired below threshold: %+v", item)
	}
	length.Store(300000)
	f.exchange(t, "private", 30003, 0, "at-threshold", false)
	warning := f.reply(t)
	if warning.Type != qq.QQMessagePrivate || warning.UserID != "30003" || !strings.Contains(qqPlainReply(warning), "建议清除上下文") || !strings.Contains(qqPlainReply(warning), "/助手 clear") {
		t.Fatalf("missing proactive host warning or wrong destination: %+v", warning)
	}
	// 等待发送成功的标志保存后再重启，避免测试本身取消仍在处理的 ACK。
	deadline := time.Now().Add(time.Second)
	for !f.contexts(t)[0].ContextWarningSent {
		if time.Now().After(deadline) {
			t.Fatal("warning state not persisted")
		}
		time.Sleep(time.Millisecond)
	}
	before := f.waitTokenStats(t, 2)
	if before.TotalTokens != 599999 {
		t.Fatal("host warning consumed a model request", before)
	}
	f.restartQQ(t)
	if stats := f.tokenStats(t); stats != before || !f.contexts(t)[0].ContextWarningSent {
		t.Fatal("restart lost cumulative usage or warning state", stats)
	}
	f.exchange(t, "private", 30003, 0, "after-restart-no-duplicate-warning", false)
	before = f.waitTokenStats(t, 3)
	record := f.contexts(t)[0]
	if err := f.core.ClearQQContext(f.clientID, f.extensionID, record.ID); err != nil {
		t.Fatal(err)
	}
	cleared := f.contexts(t)[0]
	if cleared.ContextTokens == nil || *cleared.ContextTokens != 0 || cleared.ContextWarningSent || f.tokenStats(t) != before {
		t.Fatalf("clear did not reset context length, or reset lifetime usage: %+v", cleared)
	}
	f.exchange(t, "private", 30003, 0, "warn-after-clear", false)
	if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "建议清除上下文") {
		t.Fatal("clear failed to re-enable the warning", qqPlainReply(reply))
	}
	f.exchange(t, "group", 30003, 23456, "agent-threshold", true)
	if reply := f.reply(t); reply.Type != qq.QQMessageGroup || reply.GroupID != "23456" || !strings.Contains(qqPlainReply(reply), "/助手 clear agent") {
		t.Fatalf("agent warning used the wrong context or destination: %+v", reply)
	}
	before = f.waitTokenStats(t, 5)
	if err := f.core.DeleteQQContext(f.clientID, f.extensionID, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.contexts(t)) != 0 || f.tokenStats(t) != before {
		t.Fatal("context deletion reset cumulative usage")
	}
	q, _ := f.core.Servers.Get[*backend.QQServer](backend.QQServerName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if stats := f.tokenStats(t); stats != before {
		t.Fatal("offline listing lost cumulative usage", stats)
	}
	data, err := os.ReadFile(filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID, "usage.json"))
	var persisted backend.QQTokenStats
	if err != nil || json.Unmarshal(data, &persisted) != nil || persisted != before {
		t.Fatalf("usage was not saved independently of contexts: %s / %v", data, err)
	}
}

func TestQQChatAgentAlwaysUsesDefaultPersona(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false", "agentEnabled": "true"}, nil)
	setPersona := func(id string) {
		config, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
		if err != nil {
			t.Fatal(err)
		}
		config.PersonaID = id
		if _, err := f.core.UpdateClientExtensionChatConfig(f.clientID, f.extensionID, *config); err != nil {
			t.Fatal(err)
		}
	}
	check := func(request api.ChatCompletionsRequest, id string) {
		expected, err := persona.Resolve(id).Prompt()
		if err != nil || len(request.Messages) == 0 || request.Messages[0].Role != "system" || request.Messages[0].Content != expected {
			t.Fatalf("wrong active persona, expected %s: %+v", id, request.Messages)
		}
	}
	setPersona("Iris")
	check(f.exchange(t, "private", 30003, 0, "ordinary-custom-persona", false), "Iris")
	check(f.exchange(t, "group", 30003, 23456, "agent-default-persona", true), persona.DefaultID)
	for _, item := range f.contexts(t) {
		expected := "Iris"
		if item.Kind == "agent" {
			expected = persona.DefaultID
		}
		if session := readQQSession(t, f, item.ID); session.Tree.Root.PersonaID != expected {
			t.Fatal("saved QQ persona differs from request persona")
		}
	}
	setPersona("xiaofeiyuForJieGeng")
	check(f.exchange(t, "group", 30003, 34567, "agent-still-default", true), persona.DefaultID)
	check(f.exchange(t, "private", 30003, 0, "ordinary-new-persona", false), "xiaofeiyuForJieGeng")
	f.restartQQ(t)
	check(f.exchange(t, "group", 30003, 23456, "agent-default-after-restart", true), persona.DefaultID)
}

func TestQQChatUsageAcrossConcurrentContexts(t *testing.T) {
	gate := make(chan struct{})
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		qqSSEUsage(w, "完成", &api.TokenUsage{PromptTokens: 1000, CompletionTokens: 10, TotalTokens: 1010,
			PromptTokensDetails: &api.PromptTokenDetails{CachedTokens: intPointer(100)}}, 2)
	})
	for user := 30003; user < 30007; user++ {
		f.message(t, "private", user, 0, "concurrent-context", false)
	}
	for range 4 {
		f.request(t)
	}
	close(gate)
	for range 4 {
		f.reply(t)
	}
	stats := f.waitTokenStats(t, 4)
	if stats.TotalTokens != 4040 || stats.PromptTokens != 4000 || stats.CachedPromptTokens != 400 || stats.CachePromptTokens != 4000 || stats.UsageRequests != 4 {
		t.Fatalf("concurrent request accounting lost updates: %+v", stats)
	}
	for _, item := range f.contexts(t) {
		if item.ContextTokens == nil || *item.ContextTokens != 1010 {
			t.Fatal("concurrent context length lost", item)
		}
	}
}

func TestQQChatReportedUsageIsKeptWhenStreamFails(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":30,\"total_tokens\":150}}\n\ndata: {\"error\":{\"message\":\"stream failed\"}}\n\n")
	})
	f.message(t, "private", 30003, 0, "failed-request", false)
	f.request(t)
	if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "本次回复失败") {
		t.Fatal("stream error was not reported to QQ")
	}
	if stats := f.waitTokenStats(t, 1); stats.TotalTokens != 150 || stats.UsageRequests != 1 {
		t.Fatal("reported usage was discarded after stream failure", stats)
	}
}
