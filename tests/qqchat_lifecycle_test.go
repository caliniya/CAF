package Tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
)

func TestQQChatReferencesVisibleWithoutGroupContext(t *testing.T) {
	for _, isolated := range []bool{true, false} {
		for _, whitelist := range []bool{true, false} {
			t.Run(fmt.Sprintf("isolation=%t/whitelist=%t", isolated, whitelist), func(t *testing.T) {
				f := useQQChat(t, map[string]string{"groupContext": "false", "contextIsolation": fmt.Sprint(isolated), "whitelistEnabled": fmt.Sprint(whitelist), "whitelist": "30003"}, nil)
				id := f.messageSegments(t, "group", 50005, 23456, qq.QQText("referenced-other-person-original"))
				f.messageSegments(t, "group", 30003, 23456, qq.QQMessageSegment{Type: qq.QQSegmentReply, Data: map[string]any{"id": fmt.Sprint(id)}}, qq.QQText("助手 explain-this-reference"))
				request := f.request(t)
				f.reply(t)
				visible := false
				for _, message := range request.Messages {
					if message.Role == "system" && strings.Contains(fmt.Sprint(message.Content), "referenced-other-person-original") {
						visible = true
					}
				}
				if !visible {
					t.Fatal("referenced original was hidden by whitelist, context isolation or disabled nearby context")
				}
				if len(f.contexts(t)) != 1 {
					t.Fatal("referenced sender unexpectedly acquired a chat context")
				}
			})
		}
	}
}

func TestQQChatReferenceRetrievedOverExistingConnection(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false", "whitelistEnabled": "true", "whitelist": "30003"}, nil)
	f.messageSegments(t, "group", 30003, 23456, qq.QQMessageSegment{Type: qq.QQSegmentReply, Data: map[string]any{"id": "-456"}}, qq.QQText("助手 quote-old-message"))
	action := readOneBotAction(t, f.connection)
	if action.Action != "get_msg" || fmt.Sprint(action.Params["message_id"]) != "-456" {
		t.Fatalf("reference was not looked up on the existing OneBot connection: %+v", action)
	}
	writeOneBotPacket(t, f.connection, qq.OneBot11ActionResponse{Status: "ok", Echo: action.Echo,
		Data: json.RawMessage(`{"message_id":-456,"message_type":"group","time":1795200000,"sender":{"user_id":50005,"nickname":"原作者"},"message":[{"type":"text","data":{"text":"old-reference-from-unlisted-member"}}]}`)})
	if request := f.request(t); !strings.Contains(qqRequestText(request), "old-reference-from-unlisted-member") {
		t.Fatal("get_msg result was not added to the model request")
	}
	f.reply(t)
	// 已取回的原文进入缓存，下一次引用不再发送 get_msg。
	f.messageSegments(t, "group", 30003, 23456, qq.QQMessageSegment{Type: qq.QQSegmentReply, Data: map[string]any{"id": "-456"}}, qq.QQText("助手 cached-quote"))
	f.request(t)
	f.reply(t)
}

func TestQQChatMissingReferenceDoesNotInventOriginal(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false"}, nil)
	f.messageSegments(t, "private", 30003, 0, qq.QQMessageSegment{Type: qq.QQSegmentReply, Data: map[string]any{"id": "-999"}}, qq.QQText("quote-unavailable"))
	action := readOneBotAction(t, f.connection)
	writeOneBotPacket(t, f.connection, qq.OneBot11ActionResponse{Status: "failed", RetCode: 100, Echo: action.Echo, Message: "原消息已撤回"})
	request := f.request(t)
	if !strings.Contains(qqRequestText(request), "原消息已撤回") {
		t.Fatal("missing original did not carry an explicit unavailable marker")
	}
	f.reply(t)
}

func TestQQChatDefaultProactiveJudgesEveryQueuedMessage(t *testing.T) {
	gate := make(chan struct{})
	var decisions atomic.Int32
	var addressMu sync.Mutex
	addresses := map[string]bool{}
	f := useQQChat(t, map[string]string{"proactiveEnabled": "true", "proactiveCooldown": "0", "whitelistEnabled": "true", "whitelist": "30003"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		addressMu.Lock()
		addresses[r.RemoteAddr] = true
		addressMu.Unlock()
		if decisions.Add(1) == 1 {
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		qqSSE(w, `{"reply":false}`)
	})
	parameters, err := f.core.GetClientExtensionParameters(f.clientID, f.extensionID)
	if err != nil || parameters["proactiveWidth"] != "1" {
		t.Fatalf("wrong default proactive width: %v / %v", parameters, err)
	}
	f.message(t, "group", 50005, 23456, "nearby-unlisted-visible", false)
	f.message(t, "group", 30003, 23456, "judge-first", false)
	first := f.request(t)
	if !strings.Contains(qqRequestText(first), "nearby-unlisted-visible") || len(first.Tools) != 0 {
		t.Fatal("proactive judge excluded all-member nearby context or used tools")
	}
	f.message(t, "group", 30003, 23456, "judge-second", false)
	f.message(t, "group", 30003, 23456, "judge-third", false)
	f.message(t, "group", 30003, 23456, "/助手 status", false)
	f.reply(t)
	select {
	case <-f.requests:
		t.Fatal("proactive decisions ran concurrently within one group")
	default:
	}
	close(gate)
	for _, marker := range []string{"judge-second", "judge-third"} {
		request := f.request(t)
		var input struct {
			Messages []qq.QQMessage `json:"messages"`
		}
		if err := json.Unmarshal([]byte(fmt.Sprint(request.Messages[1].Content)), &input); err != nil || len(input.Messages) != 1 || qqPlainReply(input.Messages[0]) != marker {
			t.Fatalf("queued message was dropped or batched: %+v / %v", input, err)
		}
	}
	if decisions.Load() != 3 {
		t.Fatalf("expected one judgment per message, got %d", decisions.Load())
	}
	addressMu.Lock()
	defer addressMu.Unlock()
	if len(addresses) != 1 {
		t.Fatalf("sequential judgments did not reuse the HTTP connection: %v", addresses)
	}
}

func TestQQChatProactiveReplySeesAllMembersAndReferences(t *testing.T) {
	f := useQQChat(t, map[string]string{"proactiveEnabled": "true", "proactiveCooldown": "0", "whitelistEnabled": "true", "whitelist": "30003"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if strings.Contains(qqRequestText(request), "仅输出 JSON") {
			qqSSE(w, `{"reply":true}`)
		} else {
			qqSSE(w, "参与讨论")
		}
	})
	id := f.messageSegments(t, "group", 50005, 23456, qq.QQText("all-member-reference-original"))
	f.messageSegments(t, "group", 30003, 23456, qq.QQMessageSegment{Type: qq.QQSegmentReply, Data: map[string]any{"id": fmt.Sprint(id)}}, qq.QQText("proactive-current-message"))
	for _, stage := range []string{"judge", "reply"} {
		request := f.request(t)
		if !strings.Contains(qqRequestText(request), "all-member-reference-original") || !strings.Contains(qqRequestText(request), "proactive-current-message") {
			t.Fatalf("%s lost nearby context or quoted original", stage)
		}
	}
	f.reply(t)
}

func TestQQChatDeleteRemovesWorkspaceHistoryAndCancelsQueue(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false", "longTermMemory": "true", "agentEnabled": "true"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if strings.Contains(qqRequestText(request), "delete-running-request") {
			<-r.Context().Done()
			return
		}
		qqSSE(w, "完成")
	})
	f.exchange(t, "private", 30003, 0, "context-to-delete", false)
	deleted := f.contexts(t)[0]
	f.exchange(t, "group", 30003, 23456, "agent-to-preserve", true)
	var preserved backend.QQContextInfo
	for _, item := range f.contexts(t) {
		if item.Kind == "agent" {
			preserved = item
		}
	}
	for _, item := range []backend.QQContextInfo{deleted, preserved} {
		if err := os.WriteFile(filepath.Join(item.Workspace.Path, "keep-or-delete.txt"), []byte("workspace-data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.core.ClearQQContext(f.clientID, f.extensionID, deleted.ID); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(deleted.Workspace.Path, "nested", "deep", "file.txt")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("nested-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	memoryDirectory := filepath.Join(f.core.Paths.Data, "qq", "memories", "10001", "30003")
	if _, err := os.Stat(memoryDirectory); err != nil {
		t.Fatal("personal memory missing before deletion", err)
	}
	f.message(t, "private", 30003, 0, "delete-running-request", false)
	f.request(t)
	f.message(t, "private", 30003, 0, "delete-queued-request", false)
	f.message(t, "private", 30003, 0, "/助手 status", false)
	f.reply(t)
	if err := f.core.DeleteQQContext("wrong-client", f.extensionID, deleted.ID); err == nil {
		t.Fatal("other client could delete context")
	}
	if err := f.core.DeleteQQContext(f.clientID, f.extensionID, deleted.ID); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID)
	for _, path := range []string{deleted.Workspace.Path, filepath.Join(directory, "sessions", deleted.ID+".json"), filepath.Join(directory, "history", deleted.ID)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("deleted context data still exists: %s / %v", path, err)
		}
	}
	if items := f.contexts(t); len(items) != 1 || items[0].ID != preserved.ID {
		t.Fatalf("deletion affected another context or retained the deleted record: %+v", items)
	}
	if _, err := os.Stat(filepath.Join(preserved.Workspace.Path, "keep-or-delete.txt")); err != nil {
		t.Fatal("deletion affected another workspace", err)
	}
	if _, err := os.Stat(memoryDirectory); err != nil {
		t.Fatal("deletion removed shared personal memory", err)
	}
	request := f.exchange(t, "private", 30003, 0, "new-after-delete", false)
	if strings.Contains(qqRequestText(request), "delete-running-request") || strings.Contains(qqRequestText(request), "delete-queued-request") {
		t.Fatal("deleted history or pending messages returned")
	}
	for _, item := range f.contexts(t) {
		if item.Kind == "person" && (item.ID == deleted.ID || item.Workspace.ID == deleted.Workspace.ID) {
			t.Fatal("new message reused deleted context/workspace IDs")
		}
	}
	if err := f.core.DeleteQQContext(f.clientID, f.extensionID, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.contexts(t)) != 0 {
		t.Fatal("delete-all retained contexts")
	}
}

func TestQQChatDeleteCommandRequiresConfirmationAndWorksOffline(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false"}, nil)
	f.exchange(t, "private", 30003, 0, "delete-command", false)
	before := f.contexts(t)[0]
	f.message(t, "private", 30003, 0, "/助手 delete", false)
	if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "delete confirm") || len(f.contexts(t)) != 1 {
		t.Fatal("delete command did not require explicit confirmation")
	}
	f.message(t, "private", 30003, 0, "/助手 delete confirm", false)
	if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "已删除") || len(f.contexts(t)) != 0 {
		t.Fatal("confirmed delete command failed")
	}
	f.exchange(t, "private", 30003, 0, "offline-delete", false)
	current := f.contexts(t)[0]
	if current.ID == before.ID {
		t.Fatal("deleted command context was reused")
	}
	q, _ := f.core.Servers.Get[*backend.QQServer](backend.QQServerName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.core.SetClientExtensionEnabled(f.clientID, f.extensionID, false); err != nil {
		t.Fatal(err)
	}
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 不信任持久化索引中的路径，删除目标必须重新从宿主和合法 ID 计算。
	directory := filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep-outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	current.Workspace.Path = filepath.Dir(outside)
	index, _ := json.Marshal([]backend.QQContextInfo{current})
	if err := os.WriteFile(filepath.Join(directory, "contexts.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.core.DeleteQQContext(f.clientID, f.extensionID, current.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil || len(f.contexts(t)) != 0 {
		t.Fatal("offline deletion trusted the stored workspace path or retained index", err)
	}
}

func TestRootedDeleteRejectsOutsideAndPreservesSymlinkTargets(t *testing.T) {
	fsys := &desktop.DesktopFileSystem{}
	root, outside := t.TempDir(), t.TempDir()
	for _, relative := range []string{".", "..", filepath.Join("..", "outside"), outside} {
		if err := fsys.RemoveAllInRoot(root, relative); err == nil {
			t.Fatalf("unsafe delete target accepted: %s", relative)
		}
	}
	file := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(file, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workspace", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "workspace", "link")); err != nil {
		t.Logf("symlink unavailable on this host: %v", err)
	}
	if err := fsys.RemoveAllInRoot(root, "workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("rooted deletion followed a link outside the workspace", err)
	}
}
