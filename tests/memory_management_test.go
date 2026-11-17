package Tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	memory "github.com/caliniya/caf/Memory"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
	"github.com/caliniya/caf/utils/id"
)

func TestMemoryManagementScopesVisibilityAndMainProtection(t *testing.T) {
	ctx := context.Background()
	core := useChatTaskCore(t)
	first, err := core.CreateWorkspace("项目一", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := core.CreateWorkspace("项目二", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatWorkspace(task.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatWorkspace(other.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	server, err := core.Servers.Get[*memory.MemoryServer](memory.MemoryServerName)
	if err != nil {
		t.Fatal(err)
	}
	main := mainMemoryID(t, server)

	// 已有对话只使用磁盘中的工作区，不能用另一个工作区 ID 越过可见范围。
	local, err := core.CreateMemoryLibrary(task.ID, second.ID, "  项目笔记  ", memory.ScopeWorkspace)
	if err != nil || local.Name != "项目笔记" || local.WorkspaceID != first.ID {
		t.Fatalf("new memory did not bind to the actual conversation workspace: %+v / %v", local, err)
	}
	global, err := core.CreateMemoryLibrary("", "", "通用笔记", memory.ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.CreateMemoryLibrary("", "", "无工作区", memory.ScopeWorkspace); err == nil {
		t.Fatal("workspace memory accepted no workspace")
	}
	if _, err := core.CreateMemoryLibrary("", "", "../错误目录", memory.ScopeGlobal); err == nil {
		t.Fatal("unsafe memory name accepted")
	}
	if _, err := core.CreateMemoryLibrary(task.ID, first.ID, local.Name, memory.ScopeWorkspace); err == nil {
		t.Fatal("duplicate memory name accepted")
	}

	checkVisible := func(chatID, workspaceID string, expectedLocal bool) {
		t.Helper()
		var settings *backend.ChatMemorySettings
		var err error
		if chatID == "draft" {
			settings, err = core.GetWorkspaceMemorySettings(workspaceID)
		} else {
			settings, err = core.GetChatMemorySettings(chatID)
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(settings.Libraries) == 0 || settings.Libraries[0].ID != main {
			t.Fatal("global main memory should be first")
		}
		foundLocal := false
		for _, library := range settings.Libraries {
			if library.Scope == memory.ScopeWorkspace && library.WorkspaceID != workspaceID {
				t.Fatalf("other workspace leaked into settings: %+v", library)
			}
			foundLocal = foundLocal || library.ID == local.ID
		}
		if foundLocal != expectedLocal {
			t.Fatalf("local memory visibility: got %v, want %v", foundLocal, expectedLocal)
		}
	}
	checkVisible(task.ID, first.ID, true)
	checkVisible(other.ID, second.ID, false)
	checkVisible("", "", false)
	checkVisible("draft", first.ID, true)
	checkVisible("draft", second.ID, false)
	if err := core.RenameMemoryLibrary(other.ID, first.ID, local.ID, "越权重命名"); err == nil {
		t.Fatal("another workspace could rename hidden memory")
	}
	if err := core.DeleteMemoryLibrary(other.ID, first.ID, local.ID); err == nil {
		t.Fatal("another workspace could delete hidden memory")
	}
	if err := core.SetMemoryLibraryScope(other.ID, first.ID, local.ID, memory.ScopeGlobal); err == nil {
		t.Fatal("another workspace could promote hidden memory")
	}
	if err := core.RenameMemoryLibrary(task.ID, first.ID, main, "主记忆改名"); err == nil {
		t.Fatal("main memory was renamed")
	}
	if err := core.DeleteMemoryLibrary(task.ID, first.ID, main); err == nil {
		t.Fatal("main memory was deleted")
	}
	if err := core.SetMemoryLibraryScope(task.ID, first.ID, main, memory.ScopeWorkspace); err == nil {
		t.Fatal("main memory scope changed")
	}

	if _, err := core.SetChatMemoryLibraries(task.ID, []string{main, local.ID, global.ID}); err != nil {
		t.Fatal(err)
	}
	key, err := server.AddMemory(ctx, local.ID, "旧正文", "旧标签")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateMemory(ctx, local.ID, key, "当前正文", "标签"); err != nil {
		t.Fatal(err)
	}
	before, _ := server.GetRecollection(ctx, local.ID)
	historyBefore, _ := server.History(ctx, local.ID, key)
	if err := core.RenameMemoryLibrary(task.ID, first.ID, local.ID, "  重命名笔记  "); err != nil {
		t.Fatal(err)
	}
	if err := core.SetMemoryLibraryScope(task.ID, first.ID, local.ID, memory.ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	meta, err := server.GetMeta(ctx, local.ID)
	if err != nil || meta.Name != "重命名笔记" || meta.Scope != memory.ScopeGlobal || meta.WorkspaceID != "" {
		t.Fatalf("promotion changed identity or did not persist scope: %+v / %v", meta, err)
	}
	checkVisible(other.ID, second.ID, true)
	checkVisible("", "", true)
	if _, err := core.SetChatMemoryLibraries(other.ID, []string{local.ID}); err != nil {
		t.Fatal(err)
	}
	if err := core.SetMemoryLibraryScope(task.ID, second.ID, local.ID, memory.ScopeWorkspace); err != nil {
		t.Fatal(err)
	}
	meta, _ = server.GetMeta(ctx, local.ID)
	if meta.WorkspaceID != first.ID {
		t.Fatal("scope change trusted a forged conversation workspace")
	}
	after, _ := server.GetRecollection(ctx, local.ID)
	historyAfter, _ := server.History(ctx, local.ID, key)
	if !reflect.DeepEqual(before.Data, after.Data) || !reflect.DeepEqual(historyBefore, historyAfter) {
		t.Fatal("rename/scope change modified current memories or history")
	}
	otherSettings, err := core.GetChatMemorySettings(other.ID)
	if err != nil || len(otherSettings.EnabledIDs) != 0 {
		t.Fatalf("demoted memory stayed enabled in another workspace: %+v / %v", otherSettings, err)
	}
	var stored struct {
		IDs []string `json:"memoryLibraryIds"`
	}
	memoryReadJSON(t, filepath.Join(core.Paths.Data, "chats", other.ID+".json"), &stored)
	if len(stored.IDs) != 0 {
		t.Fatal("pruned scope selection was not saved")
	}
	if err := core.DeleteMemoryLibrary(task.ID, first.ID, local.ID); err != nil {
		t.Fatal(err)
	}
	settings, err := core.GetChatMemorySettings(task.ID)
	if err != nil || slices.Contains(settings.EnabledIDs, local.ID) {
		t.Fatalf("deleted memory stayed enabled: %+v / %v", settings, err)
	}
	var archived memory.Recollection
	memoryReadJSON(t, filepath.Join(core.Paths.Data, "memory", ".archive", local.ID, "data.json"), &archived)
	if !reflect.DeepEqual(before.Data, archived.Data) {
		t.Fatal("deleting memory lost its archived content")
	}
}

func TestMemoryScopeChangesReloadWithMultipleMemoriesPerWorkspace(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	workspaceID := id.MustNew(id.Workspace)
	local, err := server.EnsureWorkspace(ctx, workspaceID, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	other, err := server.CreateRecollection(ctx, "Other", "", memory.ScopeWorkspace, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ChangeScope(ctx, other.ID, memory.ScopeGlobal, ""); err != nil {
		t.Fatal(err)
	}
	if err := server.ChangeScope(ctx, other.ID, memory.ScopeWorkspace, workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := server.ChangeScope(ctx, other.ID, "invalid", workspaceID); err == nil {
		t.Fatal("invalid scope accepted")
	}
	if err := server.ChangeScope(ctx, other.ID, memory.ScopeWorkspace, ""); err == nil {
		t.Fatal("workspace scope accepted no binding")
	}
	main := mainMemoryID(t, server)
	if err := server.ChangeScope(ctx, main, memory.ScopeWorkspace, workspaceID); err == nil {
		t.Fatal("main memory accepted workspace scope")
	}
	reloaded := memory.NewMemoryServer(data, &desktop.DesktopFileSystem{}, nil)
	if err := reloaded.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{local.ID, other.ID} {
		meta, err := reloaded.GetMeta(ctx, id)
		if err != nil || meta.Scope != memory.ScopeWorkspace || meta.WorkspaceID != workspaceID {
			t.Fatalf("multiple workspace memories did not reload: %+v / %v", meta, err)
		}
	}
}

func TestDefaultWorkspaceMemoryIsNotRecreatedAfterDeletionOrPromotion(t *testing.T) {
	for _, action := range []string{"delete", "promote"} {
		t.Run(action, func(t *testing.T) {
			core := useChatTaskCore(t)
			workspace, err := core.CreateWorkspace("项目", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			settings, err := core.GetWorkspaceMemorySettings(workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			localID := ""
			for _, library := range settings.Libraries {
				if library.Scope == memory.ScopeWorkspace {
					localID = library.ID
				}
			}
			if localID == "" {
				t.Fatal("new workspace did not receive its default memory")
			}
			if action == "delete" {
				err = core.DeleteMemoryLibrary("", workspace.ID, localID)
			} else {
				err = core.SetMemoryLibraryScope("", workspace.ID, localID, memory.ScopeGlobal)
			}
			if err != nil {
				t.Fatal(err)
			}
			task, err := core.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatWorkspace(task.ID, workspace.ID); err != nil {
				t.Fatal(err)
			}
			// 重建宿主服务模拟重启；主记忆恢复，但用户删除/提升的局部记忆不补建。
			reloaded := &backend.Core{Paths: core.Paths, FS: core.FS, Configs: core.Configs}
			reloaded.InitServers()
			settings, err = reloaded.GetWorkspaceMemorySettings(workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, library := range settings.Libraries {
				if library.Scope != memory.ScopeGlobal {
					t.Fatalf("deleted/promoted default memory was recreated: %+v", library)
				}
			}
			want := 1
			if action == "promote" {
				want = 2
			}
			if len(settings.Libraries) != want {
				t.Fatalf("wrong restored global memory count: %d", len(settings.Libraries))
			}
		})
	}
}

func TestChatWorkspaceSwitchAddsSystemMessageAndCleansMemorySelection(t *testing.T) {
	core := useChatTaskCore(t)
	first, err := core.CreateWorkspace("项目一", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := core.CreateWorkspace("项目二", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	task, err = core.SetChatWorkspace(task.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, err := task.Tree.CurrentPath()
	if err != nil || len(path) != 0 {
		t.Fatal("initial workspace choice added a message to an empty conversation")
	}
	settings, err := core.GetChatMemorySettings(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, library := range settings.Libraries {
		ids = append(ids, library.ID)
	}
	task, err = core.SetChatMemoryLibraries(task.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	user := task.Tree.Start(apiImpl.ChatMessage{Role: "user", Content: "继续开发"})
	reply := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "好的"})
	user.AddNext(reply)
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core.Paths.Data, "chats", task.ID+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	task, err = core.SetChatWorkspace(task.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, err = task.Tree.CurrentPath()
	if err != nil || len(path) != 3 || path[2].Child[0].Role != "system" || path[2].Child[0].Content != "系统消息:从「项目一」切换到「项目二」" {
		t.Fatalf("workspace switch was not recorded on the current branch: %+v / %v", path, err)
	}
	if len(task.MemoryLibraryIDs) != 1 || task.MemoryLibraryIDs[0] != settings.Libraries[0].ID {
		t.Fatal("switch must retain only enabled global memory")
	}
	task, err = core.SetChatWorkspace(task.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, _ = task.Tree.CurrentPath()
	if len(path) != 3 {
		t.Fatal("same workspace choice added a duplicate system message")
	}
	task, err = core.SetChatWorkspace(task.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := core.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	request, err := loaded.Tree.ToChatRequest(core.Configs.EnablesConfig.Model[0])
	if err != nil || request.Messages[len(request.Messages)-1].Role != "system" || !strings.Contains(request.Messages[len(request.Messages)-1].Content, "不使用工作区") {
		t.Fatalf("switch record did not persist or reach model history: %+v / %v", request, err)
	}
}
