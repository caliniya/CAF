package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	memory "github.com/caliniya/caf/Memory"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	memorytools "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool"
	backend "github.com/caliniya/caf/core/Backend"
	identity "github.com/caliniya/caf/utils/id"
)

func TestMemoryGetAndSearchCurrentIndependentCopies(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	library := mainMemoryID(t, server)
	key, err := server.AddNamedMemory(ctx, library, "Recollection Title", "old-history-only", "OriginalTag")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateMemory(ctx, library, key, "Current CONTENT", "MixedCaseLabel"); err != nil {
		t.Fatal(err)
	}
	entry, err := server.GetMemory(ctx, library, key)
	if err != nil || entry.Name != "Recollection Title" || entry.Data != "Current CONTENT" || entry.MemoryDate == "" {
		t.Fatalf("single read did not return the current record: %+v / %v", entry, err)
	}
	entry.Data, (*entry.Tag)[0] = "mutated copy", "changed copy tag"
	current, _ := server.GetMemory(ctx, library, key)
	if current.Data != "Current CONTENT" || (*current.Tag)[0] != "MixedCaseLabel" {
		t.Fatal("single read exposed mutable server state")
	}
	for _, query := range []string{"TITLE", " content ", "caseLAB"} {
		result, err := server.Search(ctx, []string{library}, query)
		if err != nil || len(result[library]) != 1 || result[library][key].Data != "Current CONTENT" {
			t.Fatalf("name/content/tag substring search failed: %s / %+v / %v", query, result, err)
		}
	}
	for _, query := range []string{"old-history-only", "OriginalTag"} {
		result, err := server.Search(ctx, []string{library}, query)
		if err != nil || len(result[library]) != 0 {
			t.Fatalf("search included archived values: %s / %+v / %v", query, result, err)
		}
	}
	result, err := server.Search(ctx, nil, "CONTENT")
	if err != nil || len(result) != 0 {
		t.Fatal("empty enabled libraries must not implicitly search Main", result, err)
	}
	for _, invalid := range []struct{ library, memory string }{
		{library, "invalid"}, {library, identity.MustNew(identity.Memory)}, {identity.MustNew(identity.Recollection), key},
	} {
		if _, err := server.GetMemory(ctx, invalid.library, invalid.memory); err == nil {
			t.Fatalf("invalid or absent memory should fail: %+v", invalid)
		}
	}
	// 查询当前记忆不依赖历史文件，损坏的历史仍由 History 显式报错。
	if err := os.WriteFile(filepath.Join(data, "memory", memory.MainName, "history.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.GetMemory(ctx, library, key); err != nil {
		t.Fatal("current read unexpectedly loaded history", err)
	}
	if _, err := server.Search(ctx, []string{library}, "CONTENT"); err != nil {
		t.Fatal("current search unexpectedly loaded history", err)
	}
	if _, err := server.History(ctx, library, key); err == nil {
		t.Fatal("damaged history should still fail an explicit history read")
	}
	parent, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := server.GetMemory(parent, library, key); !errors.Is(err, context.Canceled) {
		t.Fatal("single read ignored cancellation", err)
	}
}

func TestMemoryReadToolsRegistrationArgumentsAndCancellation(t *testing.T) {
	useToolRegistry(t)
	if err := agenttool.Add(memorytools.InitMemoryTool()); err != nil {
		t.Fatal(err)
	}
	sets := (&backend.Core{}).ListToolSets()
	if len(sets) != 1 || len(sets[0].Kits) != 4 {
		t.Fatalf("memory tools should expose four independent modules: %+v", sets)
	}
	wantModules := map[string]string{"创建记忆": "CreateMemory", "读取记忆": "ReadMemory", "搜索记忆": "SearchMemory", "列出记忆": "ListMemories"}
	for _, kit := range sets[0].Kits {
		if kit.Name != kit.ID || !kit.DefaultEnabled || len(kit.Tools) != 1 || kit.Tools[0].ID != wantModules[kit.ID] {
			t.Fatalf("each memory module should contain its own enabled tool: %+v", kit)
		}
	}
	for _, id := range []string{"ReadMemory", "SearchMemory", "ListMemories"} {
		tool := agenttool.Tools[id]
		if tool.Meta().Permission != agenttool.PermissionReadOnly || tool.Meta().DisplayContent != "result" {
			t.Fatalf("memory query must be read-only: %+v", tool.Meta())
		}
		if tool.ToType().Parameters["properties"].(map[string]any)["timeout_seconds"] == nil {
			t.Fatal("memory query is missing the optional timeout")
		}
	}
	var invoked int
	ctx := agenttool.CreateContext("", agenttool.PermissionReadOnly)
	ctx.ReadMemory = func(_ context.Context, library, key string) (string, error) {
		invoked++
		if library != "library" || key != "entry" {
			t.Error("ID arguments were not trimmed", library, key)
		}
		return `{"read":true}`, nil
	}
	ctx.SearchMemory = func(_ context.Context, library string, queries []string) (string, error) {
		invoked++
		if library != "" || len(queries) != 1 || queries[0] != "keyword" {
			t.Error("search arguments changed", library, queries)
		}
		return `{}`, nil
	}
	ctx.ListMemories = func(_ context.Context, library string) (string, error) {
		invoked++
		if library != "library" {
			t.Error("list argument changed", library)
		}
		return `{"data":{}}`, nil
	}
	for id, args := range map[string]map[string]any{
		"ReadMemory":   {"recollection_id": " library ", "memory_id": " entry "},
		"SearchMemory": {"query": " keyword "},
		"ListMemories": {"recollection_id": " library "},
	} {
		tool := agenttool.Tools[id]
		if output, err := tool.Fun(ctx, args); err != nil || !json.Valid([]byte(output)) {
			t.Fatalf("valid read-only call failed: %s / %v", output, err)
		}
		before := invoked
		for _, badContext := range []*agenttool.ToolContext{nil, agenttool.CreateContext("", 0), agenttool.CreateContext("", agenttool.PermissionReadOnly)} {
			if _, err := tool.Fun(badContext, args); err == nil {
				t.Fatal("query should reject unavailable callbacks or context")
			}
		}
		for _, invalid := range []map[string]any{nil, {}, {"recollection_id": 1, "query": false, "memory_id": []string{}}, {"recollection_id": " ", "query": " ", "memory_id": " "}} {
			if _, err := tool.Fun(ctx, invalid); err == nil {
				t.Fatalf("invalid query arguments should fail: %s / %+v", id, invalid)
			}
		}
		canceled := *ctx
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		canceled.Context = parent
		if _, err := tool.Fun(&canceled, args); !errors.Is(err, context.Canceled) {
			t.Fatal("query ignored cancellation", err)
		}
		if invoked != before {
			t.Fatal("invalid or canceled query reached the host")
		}
	}
}

func TestChatMemoryToolsCanBeControlledIndependently(t *testing.T) {
	for _, module := range []struct{ kit, id string }{
		{"创建记忆", "CreateMemory"}, {"读取记忆", "ReadMemory"}, {"搜索记忆", "SearchMemory"}, {"列出记忆", "ListMemories"},
	} {
		for _, scope := range []string{"chat", "global", "chat-override"} {
			t.Run(module.id+"/"+scope, func(t *testing.T) {
				core, _ := approvalCore(t)
				ctx := context.Background()
				if _, err := core.GetChatMemorySettings(""); err != nil {
					t.Fatal(err)
				}
				server := testServer[*memory.MemoryServer](t, core, memory.MemoryServerName)
				library := mainMemoryID(t, server)
				key, err := server.AddMemory(ctx, library, "independent-memory-marker")
				if err != nil {
					t.Fatal(err)
				}
				task, err := core.CreateChat("", "", "", "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := core.SetChatMemoryLibraries(task.ID, []string{library}); err != nil {
					t.Fatal(err)
				}
				config := &agenttool.ChatToolConfig{Permission: agenttool.PermissionFullAccess}
				if scope != "chat" {
					core.Configs.ToolNormalConfig = &agenttool.ChatToolConfig{}
					core.Configs.ToolNormalConfig.SetKit(memorytools.SetName, module.kit, boolPtr(false))
				}
				if scope == "chat" {
					config.SetKit(memorytools.SetName, module.kit, boolPtr(false))
				} else if scope == "chat-override" {
					config.SetKit(memorytools.SetName, module.kit, boolPtr(true))
				}
				data, err := json.Marshal(config)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := core.SetChatToolConfig(task.ID, string(data)); err != nil {
					t.Fatal(err)
				}
				arguments := map[string]map[string]any{
					"CreateMemory": {"recollection_id": library, "data": "new-memory"},
					"ReadMemory":   {"recollection_id": library, "memory_id": key},
					"SearchMemory": {"query": "independent-memory-marker"},
					"ListMemories": {"recollection_id": library},
				}
				requests := approvalModel(t, core,
					func(request api.ChatCompletionsRequest) []api.ToolCall {
						declared := map[string]bool{}
						for _, tool := range request.Tools {
							declared[tool.Function.Name] = true
						}
						calls := []api.ToolCall{}
						for id, args := range arguments {
							want := id != module.id || scope == "chat-override"
							if declared[id] != want {
								t.Errorf("switch affected the wrong tool declaration: %s / declared=%v / want=%v", id, declared[id], want)
							}
							// 模拟模型仍尝试调用禁用工具，执行端也必须拒绝。
							calls = append(calls, approvalCall(id, id, args))
						}
						return calls
					},
					func(request api.ChatCompletionsRequest) []api.ToolCall {
						results := map[string]string{}
						for _, message := range request.Messages {
							if message.Role == "tool" {
								results[message.ToolCallID] = fmt.Sprint(message.Content)
							}
						}
						for id := range arguments {
							output := results[id]
							if id == module.id && scope != "chat-override" {
								if output != "此工具被禁用" {
									t.Errorf("disabled memory tool was not rejected: %s / %s", id, output)
								}
							} else if id == "CreateMemory" {
								if !strings.HasPrefix(output, "已创建记忆：") {
									t.Error("enabled creation failed", output)
								}
							} else if !json.Valid([]byte(output)) || !strings.Contains(output, "independent-memory-marker") {
								t.Errorf("enabled memory query was affected by another module's switch: %s / %s", id, output)
							}
						}
						return nil
					},
				)
				if err := core.SendChatMessage(task.ID, "test independent memory tools", ""); err != nil {
					t.Fatal(err)
				}
				if requests.Load() != 2 {
					t.Fatalf("expected tool round and final response, got %d requests", requests.Load())
				}
				current, err := server.GetRecollection(ctx, library)
				if err != nil {
					t.Fatal(err)
				}
				want := 2
				if module.id == "CreateMemory" && scope != "chat-override" {
					want = 1
				}
				if len(current.Data) != want {
					t.Fatalf("creation switch did not protect persisted memory: got %d / want %d", len(current.Data), want)
				}
			})
		}
	}
}

func TestChatMemoryReadToolsRespectEnabledLibrariesAtReadOnlyPermission(t *testing.T) {
	core, _ := approvalCore(t)
	ctx := context.Background()
	settings, err := core.GetChatMemorySettings("")
	if err != nil {
		t.Fatal(err)
	}
	server := testServer[*memory.MemoryServer](t, core, memory.MemoryServerName)
	global := settings.Libraries[0].ID
	workspace, err := core.CreateWorkspace("Other project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local, err := server.EnsureWorkspace(ctx, workspace.ID, workspace.Name)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := server.CreateRecollection(ctx, "Disabled", "", memory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	globalKey, err := server.AddNamedMemory(ctx, global, "Global title", "archived-content", "global")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateMemory(ctx, global, globalKey, "current-global", "global"); err != nil {
		t.Fatal(err)
	}
	localKey, err := server.AddNamedMemory(ctx, local.ID, "CaseSensitiveName", "current-local", "SourceTag")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.AddMemory(ctx, disabled.ID, "disabled-current-marker"); err != nil {
		t.Fatal(err)
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatMemoryLibraries(task.ID, []string{global, local.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolConfig(task.ID, `{"permission":1}`); err != nil {
		t.Fatal(err)
	}
	missingMemory := identity.MustNew(identity.Memory)
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{
				approvalCall("global-read", "ReadMemory", map[string]any{"recollection_id": global, "memory_id": globalKey}),
				approvalCall("local-read", "ReadMemory", map[string]any{"recollection_id": local.ID, "memory_id": localKey}),
				approvalCall("global-list", "ListMemories", map[string]any{"recollection_id": global}),
				approvalCall("local-list", "ListMemories", map[string]any{"recollection_id": local.ID}),
				approvalCall("all-search", "SearchMemory", map[string]any{"query": "CURRENT"}),
				approvalCall("name-search", "SearchMemory", map[string]any{"recollection_id": local.ID, "query": "SENSITIVENAME"}),
				approvalCall("tag-search", "SearchMemory", map[string]any{"query": "rceTAG"}),
				approvalCall("history-search", "SearchMemory", map[string]any{"query": "archived-content"}),
				approvalCall("disabled-read", "ReadMemory", map[string]any{"recollection_id": disabled.ID, "memory_id": globalKey}),
				approvalCall("disabled-list", "ListMemories", map[string]any{"recollection_id": disabled.ID}),
				approvalCall("disabled-search", "SearchMemory", map[string]any{"recollection_id": disabled.ID, "query": "disabled"}),
				approvalCall("missing-read", "ReadMemory", map[string]any{"recollection_id": global, "memory_id": missingMemory}),
			}
		},
		func(request api.ChatCompletionsRequest) []api.ToolCall {
			results := make(map[string]string)
			for _, message := range request.Messages {
				if message.Role == "tool" {
					results[message.ToolCallID] = fmt.Sprint(message.Content)
				}
			}
			for _, id := range []string{"global-read", "local-read", "global-list", "local-list", "all-search", "name-search", "tag-search", "history-search"} {
				if !json.Valid([]byte(results[id])) {
					t.Errorf("read-only enabled library query failed: %s / %s", id, results[id])
				}
				for _, hidden := range []string{"archived-content", "disabled-current-marker"} {
					if strings.Contains(results[id], hidden) {
						t.Error("query leaked disabled or archived memory", id, results[id])
					}
				}
			}
			if !strings.Contains(results["global-read"], "current-global") || !strings.Contains(results["local-read"], "current-local") || !strings.Contains(results["global-list"], memory.MainDisplayName) {
				t.Error("read/list did not return current memory or Main display name", results)
			}
			var all, name, tag, history map[string]map[string]memory.Memory
			_ = json.Unmarshal([]byte(results["all-search"]), &all)
			_ = json.Unmarshal([]byte(results["name-search"]), &name)
			_ = json.Unmarshal([]byte(results["tag-search"]), &tag)
			_ = json.Unmarshal([]byte(results["history-search"]), &history)
			if len(all) != 2 || len(all[global]) != 1 || len(all[local.ID]) != 1 || len(name) != 1 || len(name[local.ID]) != 1 || len(tag[local.ID]) != 1 || len(history[global]) != 0 || len(history[local.ID]) != 0 {
				t.Error("search did not apply library/name/tag/current constraints", results)
			}
			for _, id := range []string{"disabled-read", "disabled-list", "disabled-search", "missing-read"} {
				if !strings.Contains(results[id], "工具执行失败") {
					t.Error("query should reject disabled or nonexistent IDs", id, results[id])
				}
			}
			return nil
		},
	)
	if err := core.SendChatMessage(task.ID, "query current memory", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatMemoryLibraries(task.ID, nil); err != nil {
		t.Fatal(err)
	}
	// 下一轮没有启用任何库，搜索结果为空，显式读取 Main 也必须失败。
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{
				approvalCall("empty-search", "SearchMemory", map[string]any{"query": "current"}),
				approvalCall("main-disabled", "ReadMemory", map[string]any{"recollection_id": global, "memory_id": globalKey}),
			}
		},
		func(request api.ChatCompletionsRequest) []api.ToolCall {
			last := request.Messages[len(request.Messages)-2:]
			if fmt.Sprint(last[0].Content) != "{}" || !strings.Contains(fmt.Sprint(last[1].Content), "未在当前对话中启用") {
				t.Error("empty selection implicitly enabled Main", last)
			}
			return nil
		},
	)
	// 清空测试客户端缓存，让下一轮使用新的模拟模型地址。
	if _, err := core.UpdateChatModel(task.ID, task.ConfigID, task.Model); err != nil {
		t.Fatal(err)
	}
	if err := core.SendChatMessage(task.ID, "no enabled memories", ""); err != nil {
		t.Fatal(err)
	}
}

func TestQQMemoryReadToolsUseCurrentSenderAndDisabledMode(t *testing.T) {
	var requests atomic.Int32
	var firstLibrary, firstMemory string
	f := useQQChat(t, map[string]string{"groupContext": "false", "longTermMemory": "true"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		index := requests.Add(1)
		if index == 1 || index == 3 {
			var library string
			for _, message := range request.Messages {
				text := fmt.Sprint(message.Content)
				if message.Role == "system" && strings.Contains(text, "内部个人长期记忆如下") {
					parts := strings.SplitN(text, "\n", 2)
					var value struct{ ID string }
					if len(parts) == 2 {
						_ = json.Unmarshal([]byte(parts[1]), &value)
						library = value.ID
					}
				}
			}
			if index == 1 {
				firstLibrary = library
				qqSSE(w, "", approvalCall("create-personal", "CreateMemory", map[string]any{"recollection_id": library, "data": "personal-query-marker", "tags": []string{"QQTag"}}))
			} else {
				qqSSE(w, "",
					approvalCall("read-own", "ReadMemory", map[string]any{"recollection_id": library, "memory_id": firstMemory}),
					approvalCall("list-own", "ListMemories", map[string]any{"recollection_id": library}),
					approvalCall("search-own", "SearchMemory", map[string]any{"query": "qqTAG"}),
				)
			}
			return
		}
		if index == 2 {
			last := request.Messages[len(request.Messages)-1]
			firstMemory = strings.TrimPrefix(fmt.Sprint(last.Content), "已创建记忆：")
		}
		if index == 5 || index == 7 {
			qqSSE(w, "",
				approvalCall("read-forbidden", "ReadMemory", map[string]any{"recollection_id": firstLibrary, "memory_id": firstMemory}),
				approvalCall("list-forbidden", "ListMemories", map[string]any{"recollection_id": firstLibrary}),
				approvalCall("search-forbidden", "SearchMemory", map[string]any{"recollection_id": firstLibrary, "query": "personal"}),
			)
			return
		}
		qqSSE(w, "完成")
	})
	for index, user := range []int{30003, 30003, 40004, 30003} {
		if index == 3 {
			f.options(t, map[string]string{"longTermMemory": "false"})
		}
		f.message(t, "private", user, 0, "query-test", false)
		f.request(t)
		request := f.request(t)
		f.reply(t)
		if index == 0 && !identity.Valid(firstMemory, identity.Memory) {
			t.Fatal("personal memory creation failed")
		}
		if index == 1 {
			for _, message := range request.Messages[len(request.Messages)-3:] {
				if message.Role != "tool" || !json.Valid([]byte(fmt.Sprint(message.Content))) || !strings.Contains(fmt.Sprint(message.Content), "personal-query-marker") {
					t.Error("ordinary QQ mode failed a personal-memory query", message)
				}
			}
		}
		if index >= 2 {
			for _, message := range request.Messages[len(request.Messages)-3:] {
				text := fmt.Sprint(message.Content)
				if message.Role != "tool" || !strings.Contains(text, "工具执行失败") || strings.Contains(text, "personal-query-marker") {
					t.Error("query leaked another user's memory or used disabled long-term memory", message)
				}
			}
		}
	}
}
