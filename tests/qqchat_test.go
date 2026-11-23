package Tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	memory "github.com/caliniya/caf/Memory"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	mainTools "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
	memoryTools "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	identity "github.com/caliniya/caf/utils/id"
	"github.com/gorilla/websocket"
)

type qqChatFixture struct {
	core                  *backend.Core
	clientID, extensionID string
	connection            *websocket.Conn
	requests              chan api.ChatCompletionsRequest
	sequence              int
}

func qqSSE(w http.ResponseWriter, content string, calls ...api.ToolCall) {
	qqSSEUsage(w, content, nil, 0, calls...)
}

func qqSSEUsage(w http.ResponseWriter, content string, usage *api.TokenUsage, snapshots int, calls ...api.ToolCall) {
	w.Header().Set("Content-Type", "text/event-stream")
	delta := map[string]any{"content": content}
	if len(calls) > 0 {
		values := []any{}
		for index, call := range calls {
			values = append(values, map[string]any{"index": index, "id": call.ID, "type": "function", "function": map[string]string{"name": call.Name, "arguments": call.Arguments}})
		}
		delta["tool_calls"] = values
	}
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": "stop"}}})
	fmt.Fprintf(w, "data: %s\n\n", body)
	if usage != nil {
		body, _ := json.Marshal(map[string]any{"choices": []any{}, "usage": usage})
		for range snapshots {
			fmt.Fprintf(w, "data: %s\n\n", body)
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func useQQChat(t *testing.T, parameters map[string]string, respond func(http.ResponseWriter, *http.Request, api.ChatCompletionsRequest)) *qqChatFixture {
	t.Helper()
	useToolRegistry(t)
	for _, set := range []agenttool.ToolsSet{mainTools.InitMainTool(), memoryTools.InitMemoryTool()} {
		if err := agenttool.Add(set); err != nil {
			t.Fatal(err)
		}
	}
	core := useQQAdapterCore(t)
	core.Paths.Root = core.Paths.Data
	core.Paths.InternalWorkspaces = filepath.Join(core.Paths.Root, "InternalWorkspaces")
	previous := backend.Platform
	backend.Platform = core
	t.Cleanup(func() { backend.Platform = previous })
	f := &qqChatFixture{core: core, clientID: "qq-test", requests: make(chan api.ChatCompletionsRequest, 64)}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request api.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		f.requests <- request
		if respond != nil {
			respond(w, r, request)
		} else {
			qqSSE(w, "完成")
		}
	}))
	t.Cleanup(model.Close)
	configs := []backend.Config{{ID: identity.MustNew(identity.Config), Name: "QQ model", ServerAddress: model.URL, APIKey: "test-token", Protocol: "chat-completions", Model: []api.Model{{ID: "qq-model"}}, EnabledModel: "qq-model"}}
	core.Configs = &backend.Configs{ALLConfig: &configs, EnablesConfig: &configs[0]}
	if _, err := core.CreateExtensionClient("QQ 测试", f.clientID); err != nil {
		t.Fatal(err)
	}
	adapter, err := core.CreateQQAdapter("助手", "test-token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	f.extensionID = adapter.ID
	if _, err := core.SetExtensionClientQQ(f.clientID, adapter.ID, "助手", false); err != nil {
		t.Fatal(err)
	}
	values, err := core.GetClientExtensionParameters(f.clientID, adapter.ID)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range parameters {
		values[key] = value
	}
	if err := core.UpdateClientExtensionParameters(f.clientID, adapter.ID, values); err != nil {
		t.Fatal(err)
	}
	config, err := core.GetClientExtensionChatConfig(f.clientID, adapter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.UpdateClientExtensionChatConfig(f.clientID, adapter.ID, *config); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetClientExtensionEnabled(f.clientID, adapter.ID, true); err != nil {
		t.Fatal(err)
	}
	f.connection = connectReverseWS(t, adapter.Address, "10001", "test-token")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		q, _ := core.Servers.Get[*backend.QQServer](backend.QQServerName)
		if err := q.Close(ctx); err != nil {
			t.Error(err)
		}
		s, _ := core.Servers.Get[*backend.ChatServer](backend.ChatServerName)
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f *qqChatFixture) message(t *testing.T, kind string, user, group int, text string, mention bool) {
	t.Helper()
	segments := []qq.QQMessageSegment{}
	if mention {
		segments = append(segments, qq.QQMessageSegment{Type: qq.QQSegmentAt, Data: map[string]any{"qq": "10001"}})
	}
	segments = append(segments, qq.QQText(text))
	f.messageSegments(t, kind, user, group, segments...)
}

func (f *qqChatFixture) messageSegments(t *testing.T, kind string, user, group int, segments ...qq.QQMessageSegment) int {
	t.Helper()
	f.sequence++
	packet := map[string]any{"post_type": "message", "message_type": kind, "sub_type": "normal", "message_id": f.sequence,
		"self_id": 10001, "user_id": user, "time": 1795200000, "message": segments, "sender": map[string]any{"user_id": user, "nickname": fmt.Sprint(user)}}
	if group > 0 {
		packet["group_id"] = group
	}
	writeOneBotPacket(t, f.connection, packet)
	return f.sequence
}

func (f *qqChatFixture) request(t *testing.T) api.ChatCompletionsRequest {
	t.Helper()
	select {
	case value := <-f.requests:
		return value
	case <-time.After(4 * time.Second):
		t.Fatal("QQ did not request the model")
		return api.ChatCompletionsRequest{}
	}
}

func (f *qqChatFixture) reply(t *testing.T) qq.QQMessage {
	t.Helper()
	action := readOneBotAction(t, f.connection)
	message, err := qq.QQMessageFromOneBot11Action(action)
	if err != nil {
		t.Fatal(err)
	}
	writeOneBotPacket(t, f.connection, qq.OneBot11ActionResponse{Status: "ok", Echo: action.Echo, Data: json.RawMessage(`{"message_id":900}`)})
	return message
}

func (f *qqChatFixture) exchange(t *testing.T, kind string, user, group int, text string, mention bool) api.ChatCompletionsRequest {
	t.Helper()
	f.message(t, kind, user, group, text, mention)
	request := f.request(t)
	reply := f.reply(t)
	if string(reply.Type) != kind || reply.GroupID != numberOrEmpty(group) || reply.UserID != numberOrEmpty(user) && kind == "private" {
		t.Fatalf("wrong QQ reply recipient: %+v", reply)
	}
	return request
}

func numberOrEmpty(value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprint(value)
}
func qqRequestText(request api.ChatCompletionsRequest) string {
	data, _ := json.Marshal(request.Messages)
	return string(data)
}
func (f *qqChatFixture) options(t *testing.T, updates map[string]string) {
	t.Helper()
	values, err := f.core.GetClientExtensionParameters(f.clientID, f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range updates {
		values[key] = value
	}
	if err := f.core.UpdateClientExtensionParameters(f.clientID, f.extensionID, values); err != nil {
		t.Fatal(err)
	}
}
func (f *qqChatFixture) contexts(t *testing.T) []backend.QQContextInfo {
	t.Helper()
	items, err := f.core.ListQQContexts(f.clientID, f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestQQChatWhitelistNearbyContextAndPrivateEntry(t *testing.T) {
	f := useQQChat(t, map[string]string{"whitelistEnabled": "true", "whitelist": "30003, 30003 40004", "groupWhitelist": "23456", "groupContextLength": "2"}, nil)
	f.message(t, "group", 30003, 99999, "助手 blocked-group", false)
	f.message(t, "private", 50005, 0, "blocked-private", false)
	f.message(t, "group", 50005, 23456, "nearby-all-member-one", false)
	f.message(t, "group", 60006, 23456, "nearby-all-member-two", false)
	request := f.exchange(t, "group", 30003, 23456, "助手 allowed-group", false)
	text := qqRequestText(request)
	for _, marker := range []string{"nearby-all-member-one", "nearby-all-member-two"} {
		if !strings.Contains(text, marker) {
			t.Errorf("nearby group context excluded %s", marker)
		}
	}
	if strings.Contains(text, "blocked-group") || strings.Contains(text, "blocked-private") {
		t.Fatal("restricted entry leaked into context")
	}
	found := false
	for _, message := range request.Messages {
		if message.Role == "system" && strings.Contains(fmt.Sprint(message.Content), "nearby-all-member-one") {
			found = true
		}
	}
	if !found {
		t.Fatal("nearby messages were not attached as system data")
	}
	private := f.exchange(t, "private", 30003, 0, "private-no-keyword", false)
	if !strings.Contains(qqRequestText(private), "allowed-group") {
		t.Fatal("isolated per-person private/group contexts did not share history")
	}
	f.options(t, map[string]string{"whitelist": "", "groupContext": "false"})
	f.message(t, "private", 30003, 0, "blocked-empty-list", false)
	select {
	case <-f.requests:
		t.Fatal("empty enabled whitelist admitted chat")
	case <-time.After(80 * time.Millisecond):
	}
	f.options(t, map[string]string{"whitelistEnabled": "false"})
	request = f.exchange(t, "private", 70007, 0, "any-private-entry", false)
	if strings.Contains(qqRequestText(request), "blocked-empty-list") {
		t.Fatal("empty enabled whitelist admitted chat")
	}
	parameters, err := f.core.GetClientExtensionParameters(f.clientID, f.extensionID)
	if err != nil || parameters["whitelistEnabled"] != "false" || parameters["groupContext"] != "false" {
		t.Fatalf("options not saved: %v / %v", parameters, err)
	}
}

func TestQQChatContextRoutingAndAgentSeparation(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false", "agentEnabled": "true"}, nil)
	f.exchange(t, "group", 30003, 23456, "助手 person-first", false)
	request := f.exchange(t, "private", 30003, 0, "person-private", false)
	if !strings.Contains(qqRequestText(request), "person-first") {
		t.Fatal("person group/private were separated")
	}
	request = f.exchange(t, "group", 40004, 23456, "助手 other-person", false)
	if strings.Contains(qqRequestText(request), "person-first") {
		t.Fatal("per-person histories mixed")
	}
	request = f.exchange(t, "group", 30003, 23456, "agent-first", true)
	if strings.Contains(qqRequestText(request), "person-first") {
		t.Fatal("agent reused ordinary context")
	}
	request = f.exchange(t, "group", 30003, 34567, "agent-next", true)
	if !strings.Contains(qqRequestText(request), "agent-first") {
		t.Fatal("agent did not remain per-person across groups")
	}
	f.options(t, map[string]string{"contextIsolation": "false", "agentEnabled": "false"})
	f.exchange(t, "group", 30003, 23456, "助手 shared-first", false)
	request = f.exchange(t, "group", 40004, 23456, "助手 shared-other", false)
	if !strings.Contains(qqRequestText(request), "shared-first") || strings.Contains(qqRequestText(request), "other-person") {
		t.Fatal("group shared routing wrong")
	}
	request = f.exchange(t, "private", 30003, 0, "separate-private-first", false)
	if strings.Contains(qqRequestText(request), "shared-first") || strings.Contains(qqRequestText(request), "person-private") {
		t.Fatal("nonisolated private reused group/person context")
	}
	request = f.exchange(t, "private", 40004, 0, "separate-private-other", false)
	if strings.Contains(qqRequestText(request), "separate-private-first") {
		t.Fatal("private senders shared a context")
	}
	f.exchange(t, "group", 30003, 23456, "mention-with-agent-disabled", true)
	request = f.exchange(t, "group", 30003, 34567, "助手 other-group-first", false)
	if strings.Contains(qqRequestText(request), "shared-first") {
		t.Fatal("groups reused shared history")
	}
	if len(f.contexts(t)) != 7 {
		t.Fatalf("unexpected context count: %+v", f.contexts(t))
	}
	if chats, err := f.core.ListChats(); err != nil || len(chats) != 0 {
		t.Fatalf("internal QQ chat appeared in desktop sidebar: %v / %v", chats, err)
	}
	if workspaces, err := f.core.ListWorkspaces(); err != nil || len(workspaces) != 0 {
		t.Fatalf("internal QQ workspace appeared in ordinary list: %v / %v", workspaces, err)
	}
}

func TestQQChatProactiveDecisionAndWhitelist(t *testing.T) {
	var decisions atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false", "whitelistEnabled": "true", "whitelist": "30003 40004", "proactiveEnabled": "true", "proactiveContextLength": "0,0", "proactiveCooldown": "0"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if len(request.Tools) == 0 && strings.Contains(qqRequestText(request), "判断是否有必要主动参与") {
			count := decisions.Add(1)
			qqSSE(w, fmt.Sprintf(`{"reply":%t}`, count > 1))
			return
		}
		qqSSE(w, "主动回复")
	})
	f.message(t, "group", 50005, 23456, "excluded-proactive-person", false)
	f.message(t, "group", 30003, 23456, "first-batch-one", false)
	request := f.request(t)
	if len(request.Tools) != 0 || strings.Contains(qqRequestText(request), "excluded-proactive-person") {
		t.Fatal("proactive decision used tools or unlisted member")
	}
	select {
	case <-f.requests:
		t.Fatal("negative proactive decision generated a reply")
	case <-time.After(80 * time.Millisecond):
	}
	if len(f.contexts(t)) != 0 {
		t.Fatal("negative proactive batch was saved into a chat")
	}
	f.message(t, "group", 30003, 23456, "second-batch-one", false)
	f.request(t)
	request = f.request(t)
	text := qqRequestText(request)
	if !strings.Contains(text, "second-batch-one") || strings.Contains(text, "first-batch-one") {
		t.Fatal("proactive chat content wrong")
	}
	if reply := f.reply(t); reply.Type != qq.QQMessageGroup || reply.GroupID != "23456" {
		t.Fatalf("proactive reply route wrong: %+v", reply)
	}
}

func TestQQChatClearCancelsQueuedReplyAndPreservesWorkspace(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if strings.Contains(qqRequestText(request), "slow-old-request") {
			<-r.Context().Done()
			return
		}
		qqSSE(w, "fresh")
	})
	f.message(t, "private", 30003, 0, "slow-old-request", false)
	f.request(t)
	items := f.contexts(t)
	if len(items) != 1 {
		t.Fatalf("missing context: %+v", items)
	}
	file := filepath.Join(items[0].Workspace.Path, "preserved.txt")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.message(t, "private", 30003, 0, "queued-old-request", false)
	f.message(t, "private", 30003, 0, "/助手 clear", false)
	reply := f.reply(t)
	if !strings.Contains(qqPlainReply(reply), "已清除") {
		t.Fatalf("clear command was blocked behind generation: %+v", reply)
	}
	request := f.exchange(t, "private", 30003, 0, "fresh-context", false)
	if strings.Contains(qqRequestText(request), "old-request") {
		t.Fatal("cleared or queued request returned in context")
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != "keep" {
		t.Fatal("clear removed workspace files")
	}
	archive := filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID, "history", items[0].ID)
	if files, err := os.ReadDir(archive); err != nil || len(files) != 1 {
		t.Fatalf("old context not archived: %v / %v", files, err)
	}
	if err := f.core.ClearQQContext("wrong-client", f.extensionID, items[0].ID); err == nil {
		t.Fatal("other client cleared QQ context")
	}
}

func qqPlainReply(message qq.QQMessage) string {
	var text strings.Builder
	for _, segment := range message.Segments {
		if segment.Type == qq.QQSegmentText {
			text.WriteString(fmt.Sprint(segment.Data["text"]))
		}
	}
	return text.String()
}

func TestQQChatPersonalMemoryAndFileTools(t *testing.T) {
	var calls atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false", "longTermMemory": "true", "agentEnabled": "true"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		index := calls.Add(1)
		if index == 1 {
			var libraryID string
			for _, message := range request.Messages {
				content := fmt.Sprint(message.Content)
				if message.Role == "system" && strings.Contains(content, "内部个人长期记忆如下") {
					parts := strings.SplitN(content, "\n", 2)
					var value struct {
						ID string `json:"id"`
					}
					if len(parts) == 2 {
						json.Unmarshal([]byte(parts[1]), &value)
						libraryID = value.ID
					}
				}
			}
			if !identity.Valid(libraryID, identity.Recollection) {
				t.Error("personal memory library not injected")
			}
			qqSSE(w, "", approvalCall("memory", "CreateMemory", map[string]any{"recollection_id": libraryID, "data": "personal-memory-marker", "tags": []string{"qq"}}), approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "internal-workspace"}))
			return
		}
		qqSSE(w, "完成")
	})
	f.message(t, "group", 30003, 23456, "store-memory", true)
	f.request(t)
	request := f.request(t)
	f.reply(t)
	if text := qqRequestText(request); strings.Contains(text, "工具执行失败") {
		t.Fatalf("QQ tools failed: %s", text)
	}
	items := f.contexts(t)
	if len(items) != 1 {
		t.Fatal("missing personal context")
	}
	if data, err := os.ReadFile(filepath.Join(items[0].Workspace.Path, "note.txt")); err != nil || string(data) != "internal-workspace" {
		t.Fatalf("file tool not confined to internal workspace: %s / %v", data, err)
	}
	var saved chat.ChatTask
	sessionPath := filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID, "sessions", items[0].ID+".json")
	memoryReadJSON(t, sessionPath, &saved)
	path, err := saved.Tree.CurrentPath()
	if err != nil || len(path[len(path)-1].ToolOperations) != 1 {
		t.Fatalf("QQ file changes must persist their recovery association: %+v / %v", path, err)
	}
	operation := path[len(path)-1].ToolOperations[0]
	if operation.CallID != "write" || operation.WorkspaceID != items[0].Workspace.ID || operation.Pending {
		t.Fatalf("wrong QQ file recovery association: %+v", operation)
	}
	var recovery map[string]any
	memoryReadJSON(t, recoveryCachePath(f.core, operation), &recovery)
	if err := f.core.ClearQQContext(f.clientID, f.extensionID, items[0].ID); err != nil {
		t.Fatal(err)
	}
	request = f.exchange(t, "private", 30003, 0, "after-clear", false)
	if !strings.Contains(qqRequestText(request), "personal-memory-marker") {
		t.Fatal("clear deleted long term memory")
	}
	request = f.exchange(t, "private", 40004, 0, "other-person", false)
	if strings.Contains(qqRequestText(request), "personal-memory-marker") {
		t.Fatal("personal memory leaked to another user")
	}
	request = f.exchange(t, "group", 30003, 23456, "agent-memory", true)
	if !strings.Contains(qqRequestText(request), "personal-memory-marker") {
		t.Fatal("agent and ordinary chat did not share personal memory")
	}
	settings, err := f.core.GetChatMemorySettings("")
	if err != nil {
		t.Fatal(err)
	}
	for _, library := range settings.Libraries {
		if library.Name == "个人记忆" {
			t.Fatal("internal personal memory visible in public list")
		}
	}
	server := memory.NewMemoryServer(filepath.Join(f.core.Paths.Data, "qq", "memories", "10001", "30003"), f.core.FS, nil)
	if err := server.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestQQChatOptionsValidationAndConfigPersistence(t *testing.T) {
	f := useQQChat(t, map[string]string{"whitelist": "030003，40004 30003", "triggerKeywords": "别名, 另一个"}, nil)
	values, err := f.core.GetClientExtensionParameters(f.clientID, f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	if values["whitelist"] != "30003, 40004" {
		t.Fatalf("QQ whitelist was not normalized: %s", values["whitelist"])
	}
	for key, value := range map[string]string{"whitelist": "-1", "groupWhitelist": "..", "proactiveContextLength": "-1,101", "groupContextLength": "101", "agentEnabled": "yes"} {
		bad := map[string]string{}
		for k, v := range values {
			bad[k] = v
		}
		bad[key] = value
		if err := f.core.UpdateClientExtensionParameters(f.clientID, f.extensionID, bad); err == nil {
			t.Fatalf("invalid option accepted: %s=%s", key, value)
		}
	}
	config, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	config.ToolConfig.Permission = agenttool.PermissionFullAccess
	if _, err := f.core.UpdateClientExtensionChatConfig(f.clientID, f.extensionID, *config); err != nil {
		t.Fatal(err)
	}
	config, err = f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
	if err != nil || config.ToolConfig.Permission != agenttool.PermissionAutoEdit {
		t.Fatal("QQ config allowed elevated default permission")
	}
	if _, err := f.core.SetClientExtensionEnabled(f.clientID, f.extensionID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetClientExtensionEnabled(f.clientID, f.extensionID, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
	if err != nil || loaded.ConfigID != config.ConfigID || loaded.Model != config.Model {
		t.Fatal("enable toggle discarded chat config")
	}
	data, err := os.ReadFile(filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "client.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "test-token") {
		t.Fatal("extension conversation config duplicated model credential")
	}
	var saved struct {
		Extensions []struct {
			Chat *backend.ExtensionChatConfig `json:"chat"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(data, &saved); err != nil || saved.Extensions[0].Chat == nil {
		t.Fatal("chat config not persisted")
	}
}

type qqWorkspaceProbe struct {
	agenttool.ToolMeta
	executions atomic.Int32
}

func (p *qqWorkspaceProbe) Load() {
	p.ToolMeta = agenttool.ToolMeta{ID: "QQWorkspaceProbe", Name: "内部工作区申请测试", Enabled: true, Permission: agenttool.PermissionFullAccess}
}
func (p *qqWorkspaceProbe) Meta() *agenttool.ToolMeta { return &p.ToolMeta }
func (p *qqWorkspaceProbe) PermissionFor(*agenttool.ToolContext, map[string]any) (agenttool.ToolPermission, error) {
	return agenttool.ToolPermission{Level: agenttool.PermissionFullAccess, Workspace: true, Reason: "该内部工作区操作需要申请"}, nil
}
func (p *qqWorkspaceProbe) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	if !ctx.WorkspaceOnly || ctx.WorkSpace == "" || !ctx.Check(agenttool.PermissionFullAccess) {
		return "", fmt.Errorf("workspace grant missing")
	}
	p.executions.Add(1)
	return "workspace-probe-executed", nil
}

func TestQQChatStrictApprovalAndQuestions(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprint(strict), func(t *testing.T) {
			var rounds atomic.Int32
			f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
				index := rounds.Add(1)
				if index == 1 {
					qqSSE(w, "", approvalCall("original", "QQWorkspaceProbe", map[string]any{}))
					return
				}
				if strict && index == 2 {
					if !strings.Contains(qqRequestText(request), "严格模式下此调用尚未执行") {
						t.Error("strict mode did not block original tool")
					}
					qqSSE(w, "", approvalCall("request-original", agenttool.RequestToolExecutionID, map[string]any{"call_id": "original"}))
					return
				}
				last := request.Messages[len(request.Messages)-1]
				if last.Role == "user" && strings.Contains(fmt.Sprint(last.Content), "probe-again") {
					qqSSE(w, "", approvalCall("again", "QQWorkspaceProbe", map[string]any{}))
					return
				}
				if last.Role == "user" && strings.Contains(fmt.Sprint(last.Content), "ask-question") {
					qqSSE(w, "", approvalCall("question", "AskUser", map[string]any{"question": "请选择一个颜色", "options": []string{"红色", "绿色"}}))
					return
				}
				qqSSE(w, "完成")
			})
			probe := &qqWorkspaceProbe{}
			probe.Load()
			agenttool.ToolSets["主工具集"].Get("文件操作工具").ListName()[probe.ID] = probe
			agenttool.Tools[probe.ID] = probe
			agenttool.Sync()
			config, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
			if err != nil {
				t.Fatal(err)
			}
			config.ToolConfig.StrictMode = &strict
			if _, err := f.core.UpdateClientExtensionChatConfig(f.clientID, f.extensionID, *config); err != nil {
				t.Fatal(err)
			}
			f.message(t, "group", 30003, 23456, "助手 probe-first", true)
			request := f.request(t)
			declared := false
			for _, tool := range request.Tools {
				if tool.Function.Name == agenttool.RequestToolExecutionID {
					declared = true
				}
			}
			if declared != strict {
				t.Fatal("strict request tool declaration wrong")
			}
			if strict {
				f.request(t)
			}
			prompt := qqPlainReply(f.reply(t))
			id := regexp.MustCompile(`request-[0-9a-f]{32}`).FindString(prompt)
			if id == "" || !strings.Contains(prompt, "申请执行工具") || probe.executions.Load() != 0 {
				t.Fatalf("approval was not requested before execution: %s", prompt)
			}
			f.message(t, "group", 40004, 23456, "/助手 approve "+id+" workspace", false)
			if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "原请求的用户") {
				t.Fatal("other QQ member approved tool")
			}
			f.message(t, "group", 30003, 23456, "/助手 approve "+id+" workspace", false)
			f.request(t)
			first, second := qqPlainReply(f.reply(t)), qqPlainReply(f.reply(t))
			if !strings.Contains(first+second, "已提交") || !strings.Contains(first+second, "完成") {
				t.Fatalf("approval acknowledgement/final response missing: %s / %s", first, second)
			}
			if probe.executions.Load() != 1 {
				t.Fatal("approved original call not executed once")
			}
			f.message(t, "group", 30003, 23456, "助手 probe-again", true)
			f.request(t)
			request = f.request(t)
			if !strings.Contains(qqRequestText(request), "workspace-probe-executed") {
				t.Fatal("persistent workspace grant was discarded")
			}
			if reply := f.reply(t); qqPlainReply(reply) != "完成" || probe.executions.Load() != 2 {
				t.Fatal("workspace grant requested approval again")
			}
			f.message(t, "group", 30003, 23456, "助手 ask-question", true)
			f.request(t)
			question := qqPlainReply(f.reply(t))
			id = regexp.MustCompile(`request-[0-9a-f]{32}`).FindString(question)
			if id == "" || !strings.Contains(question, "请选择一个颜色") {
				t.Fatal("model question did not reach QQ")
			}
			f.message(t, "private", 30003, 0, "/助手 answer "+id+" 错误聊天", false)
			if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "原聊天") {
				t.Fatal("answer was accepted from a different channel")
			}
			f.message(t, "group", 30003, 23456, "/助手 answer "+id+" 绿色", false)
			request = f.request(t)
			if !strings.Contains(qqRequestText(request), "绿色") {
				t.Fatal("QQ answer did not return to model")
			}
			f.reply(t)
			f.reply(t)
		})
	}
}

func TestQQChatRejectsWritesOutsideInternalWorkspace(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.txt")
	var rounds atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if rounds.Add(1) == 1 {
			qqSSE(w, "", approvalCall("outside", "WriteFile", map[string]any{"path": []string{outside}, "content": "blocked"}))
			return
		}
		qqSSE(w, "完成")
	})
	f.message(t, "private", 30003, 0, "write-outside", true)
	f.request(t)
	request := f.request(t)
	if !strings.Contains(qqRequestText(request), "QQ 对话只允许") {
		t.Fatal("outside write was not refused")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("QQ tool wrote outside its workspace")
	}
	if qqPlainReply(f.reply(t)) != "完成" {
		t.Fatal("outside tool approval leaked into QQ flow")
	}
}

func TestQQChatRestartReusesPersistedContext(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false"}, nil)
	f.exchange(t, "private", 30003, 0, "persist-before-restart", false)
	before := f.contexts(t)[0]
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
	request := f.exchange(t, "private", 30003, 0, "persist-after-restart", false)
	if !strings.Contains(qqRequestText(request), "persist-before-restart") {
		t.Fatal("QQ restart lost context history")
	}
	after := f.contexts(t)[0]
	if after.ID != before.ID || after.Workspace.ID != before.Workspace.ID {
		t.Fatal("QQ restart recreated context/workspace")
	}
	if _, err := f.core.SetClientExtensionEnabled(f.clientID, f.extensionID, false); err != nil {
		t.Fatal(err)
	}
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.core.ClearQQContext(f.clientID, f.extensionID, before.ID); err != nil {
		t.Fatal(err)
	}
	if session := readQQSession(t, f, before.ID); len(session.Tree.Root.Next) != 0 {
		t.Fatal("disabled QQ context could not be cleared")
	}
}

func TestQQChatConfigChangeCancelsGenerationAndPreservesQueueOrder(t *testing.T) {
	var rounds atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if rounds.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		qqSSE(w, "完成")
	})
	f.message(t, "private", 30003, 0, "old-running-request", false)
	f.request(t)
	f.message(t, "private", 30003, 0, "old-queued-request", false)
	f.message(t, "private", 30003, 0, "/助手 status", false)
	if reply := f.reply(t); !strings.Contains(qqPlainReply(reply), "客户端：") {
		t.Fatal("queued message barrier failed")
	}
	config, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	config.ReasoningEffort = "low"
	if _, err := f.core.UpdateClientExtensionChatConfig(f.clientID, f.extensionID, *config); err != nil {
		t.Fatal(err)
	}
	f.message(t, "private", 30003, 0, "new-first-request", false)
	f.message(t, "private", 30003, 0, "new-second-request", false)
	first := f.request(t)
	f.reply(t)
	second := f.request(t)
	f.reply(t)
	firstText, secondText := qqRequestText(first), qqRequestText(second)
	if !strings.Contains(firstText, "new-first-request") || strings.Contains(firstText, "new-second-request") || !strings.Contains(secondText, "new-first-request") || !strings.Contains(secondText, "new-second-request") {
		t.Fatal("new messages lost order across config change")
	}
	if strings.Contains(firstText, "old-queued-request") || strings.Contains(secondText, "old-queued-request") {
		t.Fatal("stale queue survived config change")
	}
	if !strings.Contains(firstText, "old-running-request") {
		t.Fatal("configuration change unexpectedly cleared saved history")
	}
}

func TestQQChatDeletingClientArchivesInFlightContext(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) { <-r.Context().Done() })
	f.message(t, "private", 30003, 0, "inflight-client-delete", false)
	f.request(t)
	items := f.contexts(t)
	if len(items) != 1 {
		t.Fatal("context not created")
	}
	if err := f.core.DeleteExtensionClient(f.clientID); err != nil {
		t.Fatal(err)
	}
	q, _ := f.core.Servers.Get[*backend.QQServer](backend.QQServerName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID)); !os.IsNotExist(err) {
		t.Fatal("old generation recreated deleted client data")
	}
	archives, err := os.ReadDir(filepath.Join(f.core.Paths.Data, "ext", "archive", "clients"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("client archive missing: %v / %v", archives, err)
	}
	file := filepath.Join(f.core.Paths.Data, "ext", "archive", "clients", archives[0].Name(), "qq", f.extensionID, "sessions", items[0].ID+".json")
	if data, err := os.ReadFile(file); err != nil || !strings.Contains(string(data), "inflight-client-delete") {
		t.Fatal("archive lost in-flight QQ session")
	}
	if _, err := f.core.CreateExtensionClient("Fresh", f.clientID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetExtensionClientQQ(f.clientID, f.extensionID, "助手", true); err != nil {
		t.Fatal(err)
	}
	if contexts := f.contexts(t); len(contexts) != 0 {
		t.Fatal("new client inherited archived contexts")
	}
}

func TestQQChatProactiveDoesNotQueueDuplicateBatches(t *testing.T) {
	var decisions atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false", "proactiveEnabled": "true", "proactiveCooldown": "60"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if len(request.Tools) == 0 && strings.Contains(qqRequestText(request), "判断是否有必要主动参与") {
			decisions.Add(1)
			qqSSE(w, `{"reply":true}`)
			return
		}
		<-r.Context().Done()
	})
	f.message(t, "group", 30003, 23456, "proactive-first", false)
	f.request(t)
	f.request(t)
	f.message(t, "group", 30003, 23456, "proactive-second", false)
	f.message(t, "group", 40004, 23456, "proactive-third", false)
	f.message(t, "group", 30003, 23456, "/助手 status", false)
	if qqPlainReply(f.reply(t)) == "" {
		t.Fatal("command blocked by proactive generation")
	}
	if decisions.Load() != 1 {
		t.Fatal("a pending proactive reply allowed duplicate decisions")
	}
	if err := f.core.ClearQQContext(f.clientID, f.extensionID, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.requests:
		t.Fatal("duplicate proactive batch reached model")
	default:
	}
}

func readQQSession(t *testing.T, f *qqChatFixture, id string) chat.ChatTask {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID, "sessions", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var value chat.ChatTask
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
