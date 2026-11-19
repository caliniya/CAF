package Tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	memory "github.com/caliniya/caf/Memory"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agent "github.com/caliniya/caf/core/Agent"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	memorytool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool"
	create "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool/CreateMemory"
	backend "github.com/caliniya/caf/core/Backend"
)

func TestChatMemorySettingsPersistAndRemoveMissingIDs(t *testing.T) {
	ctx := context.Background()
	core := useChatTaskCore(t)
	settings, err := core.GetChatMemorySettings("")
	if err != nil || len(settings.EnabledIDs) != 0 || len(settings.Libraries) != 1 {
		t.Fatalf("initial settings must enable no libraries: %+v / %v", settings, err)
	}
	server, err := core.Servers.Get[*memory.MemoryServer](memory.MemoryServerName)
	if err != nil {
		t.Fatal(err)
	}
	workspace1, err := core.CreateWorkspace("Project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace2, err := core.CreateWorkspace("Project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local1, err := server.EnsureWorkspace(ctx, workspace1.ID, workspace1.Name)
	if err != nil {
		t.Fatal(err)
	}
	local2, err := server.EnsureWorkspace(ctx, workspace2.ID, workspace2.Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, workspaceID := range []string{"", workspace1.ID, workspace2.ID} {
		all, err := core.GetWorkspaceMemorySettings(workspaceID)
		if err != nil || len(all.Libraries) != 3 || len(all.EnabledIDs) != 0 {
			t.Fatalf("draft settings must list every library without enabling them: %+v / %v", all, err)
		}
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatWorkspace(task.ID, workspace1.ID); err != nil {
		t.Fatal(err)
	}
	global := settings.Libraries[0].ID
	selected := []string{global, local1.ID, local2.ID}
	updated, err := core.SetChatMemoryLibraries(task.ID, append(selected, global, "missing"))
	if err != nil || !reflect.DeepEqual(updated.MemoryLibraryIDs, selected) {
		t.Fatalf("all selected library IDs should persist, duplicates/missing removed: %+v / %v", updated, err)
	}
	var persisted map[string]json.RawMessage
	path := filepath.Join(core.Paths.Data, "chats", task.ID+".json")
	memoryReadJSON(t, path, &persisted)
	var ids []string
	if err := json.Unmarshal(persisted["memoryLibraryIds"], &ids); err != nil || !reflect.DeepEqual(ids, selected) {
		t.Fatalf("selection not persisted: %v / %v", ids, err)
	}
	if err := server.DeleteRecollection(ctx, local1.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := core.GetChat(task.ID)
	if err != nil || !reflect.DeepEqual(loaded.MemoryLibraryIDs, []string{global, local2.ID}) {
		t.Fatalf("deleted library was not pruned from session: %+v / %v", loaded, err)
	}
	memoryReadJSON(t, path, &persisted)
	_ = json.Unmarshal(persisted["memoryLibraryIds"], &ids)
	if !reflect.DeepEqual(ids, []string{global, local2.ID}) {
		t.Fatal("pruning was not saved to disk")
	}
	other, err := core.CreateChat("", "", "", "")
	if err != nil || len(other.MemoryLibraryIDs) != 0 {
		t.Fatal("new chat inherited memory settings")
	}
	if _, err := core.SetChatMemoryLibraries(task.ID, nil); err != nil {
		t.Fatal(err)
	}
	persisted = nil
	memoryReadJSON(t, path, &persisted)
	if _, exists := persisted["memoryLibraryIds"]; exists {
		t.Fatal("disabled libraries should be omitted")
	}
	if _, err := os.Stat(filepath.Join(core.Paths.Data, "memory", local2.Name, "data.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCreateMemoryToolEnforcesEnabledLibrariesAndExactGlobalPermission(t *testing.T) {
	for _, test := range []struct {
		name       string
		permission agenttool.PermissionLevel
		scope      string
		enabled    bool
		allowed    bool
	}{
		{"read_only_local", 1, memory.ScopeWorkspace, true, false},
		{"auto_edit_local", 2, memory.ScopeWorkspace, true, true},
		{"auto_approve_local", 3, memory.ScopeWorkspace, true, true},
		{"full_local", 4, memory.ScopeWorkspace, true, true},
		{"full_other_workspace", 4, memory.ScopeWorkspace, true, false},
		{"auto_edit_global", 2, memory.ScopeGlobal, true, false},
		{"auto_approve_global", 3, memory.ScopeGlobal, true, false},
		{"auto_approve_other_global", 3, memory.ScopeGlobal, true, false},
		{"full_global", 4, memory.ScopeGlobal, true, true},
		{"full_disabled_global", 4, memory.ScopeGlobal, false, false},
		{"full_disabled_local", 4, memory.ScopeWorkspace, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			useToolRegistry(t)
			if err := agenttool.Add(memorytool.InitMemoryTool()); err != nil {
				t.Fatal(err)
			}
			core := useChatTaskCore(t)
			settings, err := core.GetChatMemorySettings("")
			if err != nil {
				t.Fatal(err)
			}
			memories, _ := core.Servers.Get[*memory.MemoryServer](memory.MemoryServerName)
			libraryID := settings.Libraries[0].ID
			workspaceID := ""
			if test.name == "auto_approve_other_global" {
				global, err := memories.CreateRecollection(context.Background(), "OtherGlobal", "", memory.ScopeGlobal, "")
				if err != nil {
					t.Fatal(err)
				}
				libraryID = global.ID
			}
			if test.scope == memory.ScopeWorkspace {
				workspace, err := core.CreateWorkspace("local", t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				workspaceID = workspace.ID
				local, err := memories.EnsureWorkspace(context.Background(), workspace.ID, workspace.Name)
				if err != nil {
					t.Fatal(err)
				}
				libraryID = local.ID
				if test.name == "full_other_workspace" {
					other, err := core.CreateWorkspace("other", t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					workspaceID = other.ID
				}
			}
			task, err := core.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if workspaceID != "" {
				if _, err := core.SetChatWorkspace(task.ID, workspaceID); err != nil {
					t.Fatal(err)
				}
			}
			if test.enabled {
				if _, err := core.SetChatMemoryLibraries(task.ID, []string{libraryID}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := core.SetChatToolConfig(task.ID, fmt.Sprintf(`{"permission":%d}`, test.permission)); err != nil {
				t.Fatal(err)
			}
			calls := 0
			arguments, _ := json.Marshal(map[string]any{"recollection_id": libraryID, "data": "测试长期记忆", "tags": []string{"test"}})
			toolResponse, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{
				"tool_calls": []any{map[string]any{"index": 0, "id": "memory-call", "type": "function", "function": map[string]string{"name": "CreateMemory", "arguments": string(arguments)}}},
			}, "finish_reason": "tool_calls"}}})
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request apiImpl.ChatCompletionsRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if calls == 1 && test.enabled {
					found := false
					for _, message := range request.Messages {
						content, _ := message.Content.(string)
						if message.Role == "system" && strings.Contains(content, libraryID) {
							found = true
						}
					}
					if !found {
						t.Error("model context omitted an enabled memory library")
					}
				}
				if calls == 2 {
					last := request.Messages[len(request.Messages)-1]
					content, _ := last.Content.(string)
					if last.Role != "tool" || last.ToolCallID != "memory-call" || strings.Contains(content, "已创建记忆") != test.allowed {
						t.Errorf("wrong memory write result: %+v", last)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if calls == 1 {
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", toolResponse)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer model.Close()
			core.Configs.EnablesConfig.ServerAddress = model.URL
			previousEmit := backend.Emit
			t.Cleanup(func() { backend.Emit = previousEmit })
			backend.Emit = func(event string, data ...any) {
				if event != backend.EventChatTool {
					return
				}
				var payload map[string]string
				if err := json.Unmarshal([]byte(data[0].(string)), &payload); err != nil {
					t.Error(err)
					return
				}
				if payload["approval"] != "" && payload["requestId"] != "" {
					if test.allowed {
						t.Error("已有权限的操作不应要求审批")
					}
					if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], backend.ApprovalReject); err != nil {
						t.Error(err)
					}
				}
			}
			if err := core.SendChatMessage(task.ID, "记住这件事", "high"); err != nil {
				t.Fatal(err)
			}
			library, err := memories.GetRecollection(context.Background(), libraryID)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if test.allowed {
				want = 1
			}
			if len(library.Data) != want {
				t.Fatalf("permission check did not protect persisted data: %d memories", len(library.Data))
			}
			if calls != 2 {
				t.Fatalf("expected tool round and final response, got %d", calls)
			}
		})
	}
}

func TestCreateMemoryToolValidatesArgumentsAndCancellation(t *testing.T) {
	tool := create.InitCreateMemory().ListName()["CreateMemory"]
	calls := 0
	ctx := agenttool.CreateContext("", agenttool.PermissionAutoEdit)
	ctx.CreateMemory = func(_ *agenttool.ToolContext, library, data string, tags []string) (string, error) {
		calls++
		if library != "library" || data != "body" || !reflect.DeepEqual(tags, []string{"tag"}) {
			t.Error("arguments changed")
		}
		return "memory-id", nil
	}
	for _, args := range []map[string]any{
		{}, {"recollection_id": "library", "data": " "}, {"recollection_id": 1, "data": "body"},
		{"recollection_id": "library", "data": "body", "tags": nil},
		{"recollection_id": "library", "data": "body", "tags": []any{3}},
	} {
		if _, err := tool.Fun(ctx, args); err == nil {
			t.Fatalf("invalid args accepted: %+v", args)
		}
	}
	if calls != 0 {
		t.Fatal("invalid arguments reached host write callback")
	}
	if _, err := tool.Fun(ctx, map[string]any{"recollection_id": "library", "data": "body", "tags": []any{"tag"}}); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Context = cancelled
	if _, err := tool.Fun(ctx, map[string]any{"recollection_id": "library", "data": "body"}); err == nil {
		t.Fatal("cancelled call wrote memory")
	}
	if calls != 1 {
		t.Fatal("cancelled call reached host write callback")
	}
}

func TestMemoryToolRegisteredAtApplicationStartup(t *testing.T) {
	useToolRegistry(t)
	agent.InitTool()
	if agenttool.Tools["CreateMemory"] == nil || agenttool.ToolList["CreateMemory"].Name != "CreateMemory" {
		t.Fatal("memory tool was not registered")
	}
}
