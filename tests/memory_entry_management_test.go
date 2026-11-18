package Tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	memory "github.com/caliniya/caf/Memory"
	backend "github.com/caliniya/caf/core/Backend"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
	"github.com/caliniya/caf/utils/id"
)

func TestMemoryEntryManagementPreservesNamesDatesAndHistory(t *testing.T) {
	ctx := context.Background()
	core := useChatTaskCore(t)
	workspace, err := core.CreateWorkspace("编辑项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings, err := core.GetWorkspaceMemorySettings(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	server, err := core.Servers.Get[*memory.MemoryServer](memory.MemoryServerName)
	if err != nil {
		t.Fatal(err)
	}
	for _, library := range settings.Libraries {
		t.Run(library.Scope, func(t *testing.T) {
			// 人工管理允许编辑未启用的库，包括其他工作区和全局主记忆。
			details, err := core.CreateMemoryEntry("", "", library.ID, backend.MemoryEntryInput{
				Name: "  原名称  ", Data: "原正文\n第二行", Tags: []string{"Go", "原标签"}, Date: "ignored-on-create",
			})
			if err != nil || len(details.Data) != 1 {
				t.Fatalf("could not create an entry: %+v / %v", details, err)
			}
			key := ""
			var original memory.Memory
			for memoryID, entry := range details.Data {
				key, original = memoryID, entry
			}
			if !id.Valid(key, id.Memory) || original.Name != "原名称" || original.Data != "原正文\n第二行" || original.MemoryDate != details.Date {
				t.Fatalf("creation changed identity, name, content, or time: %+v", original)
			}
			if _, err := time.Parse(time.RFC3339Nano, original.MemoryDate); err != nil {
				t.Fatal(err)
			}
			history, err := server.History(ctx, library.ID, key)
			if err != nil || len(history) != 0 {
				t.Fatal("creation should not add a history snapshot")
			}
			input := backend.MemoryEntryInput{Name: "新名称", Data: "新正文", Date: "2026-11-17T10:12:13.123456789+08:00", Tags: []string{"新标签"}}
			details, err = core.UpdateMemoryEntry("", "", library.ID, key, input)
			if err != nil {
				t.Fatal(err)
			}
			current := details.Data[key]
			if current.Name != input.Name || current.MemoryDate != input.Date || current.Data != input.Data || !reflect.DeepEqual([]string(*current.Tag), input.Tags) {
				t.Fatalf("manual editing lost a field or overwrote its time: %+v", current)
			}
			history, err = server.History(ctx, library.ID, key)
			if err != nil || len(history) != 1 || !reflect.DeepEqual(history[0], original) {
				t.Fatalf("edit did not preserve the complete previous entry: %+v / %v", history, err)
			}
			if _, err := core.UpdateMemoryEntry("", "", library.ID, key, input); err != nil {
				t.Fatal(err)
			}
			history, _ = server.History(ctx, library.ID, key)
			if len(history) != 1 {
				t.Fatal("saving an unchanged entry duplicated its history")
			}
			// 仅编辑时间也需要保留原副本；不新增版本或其他时间字段。
			input.Date = "2026-11-16T07:08:09Z"
			if _, err := core.UpdateMemoryEntry("", "", library.ID, key, input); err != nil {
				t.Fatal(err)
			}
			history, _ = server.History(ctx, library.ID, key)
			if len(history) != 2 || !reflect.DeepEqual(history[1], current) {
				t.Fatal("changing only the time lost the previous snapshot")
			}
			var wire struct {
				Data map[string]map[string]json.RawMessage `json:"data"`
			}
			memoryReadJSON(t, filepath.Join(core.Paths.Data, "memory", library.Name, "data.json"), &wire)
			if fields := wire.Data[key]; len(fields) != 4 || fields["name"] == nil || fields["date"] == nil || fields["data"] == nil || fields["description"] == nil {
				t.Fatalf("unexpected entry storage fields: %+v", fields)
			}
			reloaded := memory.NewMemoryServer(core.Paths.Data, &desktop.DesktopFileSystem{}, nil)
			if err := reloaded.Init(ctx); err != nil {
				t.Fatal(err)
			}
			loaded, err := reloaded.GetRecollection(ctx, library.ID)
			if err != nil || loaded.Data[key].MemoryDate != input.Date || loaded.Data[key].Name != input.Name {
				t.Fatalf("name/time did not survive reload: %+v / %v", loaded, err)
			}
			details, err = core.DeleteMemoryEntry("", "", library.ID, key)
			if err != nil || len(details.Data) != 0 {
				t.Fatalf("entry deletion failed: %+v / %v", details, err)
			}
			history, _ = server.History(ctx, library.ID, key)
			if len(history) != 3 || history[2].Name != input.Name || history[2].MemoryDate != input.Date || history[2].Data != input.Data {
				t.Fatalf("deletion lost the last entry: %+v", history)
			}
		})
	}
}

func TestMemoryEntryRejectsInvalidEditsAndCorruptHistory(t *testing.T) {
	core := useChatTaskCore(t)
	settings, err := core.GetChatMemorySettings("")
	if err != nil {
		t.Fatal(err)
	}
	library := settings.Libraries[0]
	input := backend.MemoryEntryInput{Name: "名称", Data: "正文", Tags: []string{}}
	details, err := core.CreateMemoryEntry("", "", library.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for memoryID, entry := range details.Data {
		key, input.Date = memoryID, entry.MemoryDate
	}
	for _, invalid := range []backend.MemoryEntryInput{
		{Name: " ", Data: "正文", Date: input.Date},
		{Name: "名称", Data: " \n ", Date: input.Date},
		{Name: "名称", Data: "正文", Date: "invalid"},
		{Name: "名称", Data: "正文", Date: "2026-02-30T08:00:00Z"},
	} {
		if _, err := core.UpdateMemoryEntry("", "", library.ID, key, invalid); err == nil {
			t.Fatalf("invalid edit accepted: %+v", invalid)
		}
	}
	if _, err := core.UpdateMemoryEntry("", "", library.ID, id.MustNew(id.Memory), input); err == nil {
		t.Fatal("editing a missing memory created a new entry")
	}
	if _, err := core.DeleteMemoryEntry("", "", library.ID, "missing"); err == nil {
		t.Fatal("deleting a missing memory succeeded")
	}
	if _, err := core.CreateMemoryEntry("", "", "missing", input); err == nil {
		t.Fatal("creation accepted a missing library")
	}
	if err := os.WriteFile(filepath.Join(core.Paths.Data, "memory", library.Name, "history.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	input.Name = "新名称"
	if _, err := core.UpdateMemoryEntry("", "", library.ID, key, input); err == nil {
		t.Fatal("editing overwrote corrupt history")
	}
	if _, err := core.DeleteMemoryEntry("", "", library.ID, key); err == nil {
		t.Fatal("deleting overwrote corrupt history")
	}
	after, err := core.GetMemoryLibraryDetails("", "", library.ID)
	if err != nil || !reflect.DeepEqual(after.Data, details.Data) {
		t.Fatalf("failed operations changed current entries: %+v / %v", after, err)
	}
}

func TestNamedMemoryToolUpdatesKeepNameAndRestoreIt(t *testing.T) {
	server, _ := testMemoryServer(t)
	ctx := context.Background()
	libraryID := mainMemoryID(t, server)
	key, err := server.AddNamedMemory(ctx, libraryID, "原名称", "正文", "标签")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateMemory(ctx, libraryID, key, "工具更新", "标签"); err != nil {
		t.Fatal(err)
	}
	r, _ := server.GetRecollection(ctx, libraryID)
	if r.Data[key].Name != "原名称" {
		t.Fatal("ordinary updates discarded the memory name")
	}
	next := r.Data[key]
	next.Name, next.MemoryDate = "编辑名称", "2026-11-17T01:02:03Z"
	if err := server.EditMemory(ctx, libraryID, key, next); err != nil {
		t.Fatal(err)
	}
	if err := server.RestoreMemory(ctx, libraryID, key, 0); err != nil {
		t.Fatal(err)
	}
	r, _ = server.GetRecollection(ctx, libraryID)
	if r.Data[key].Name != "原名称" || r.Data[key].Data != "正文" {
		t.Fatal("history restoration lost a memory name")
	}
}
