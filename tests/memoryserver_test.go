package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	memory "github.com/caliniya/caf/Memory"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	platform "github.com/caliniya/caf/platform"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
	"github.com/caliniya/caf/utils/id"
)

var _ backend.Server = (*memory.MemoryServer)(nil)

func testMemoryServer(t *testing.T) (*memory.MemoryServer, string) {
	t.Helper()
	data := t.TempDir()
	server := memory.NewMemoryServer(data, &desktop.DesktopFileSystem{}, nil)
	if err := server.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return server, data
}

func mainMemoryID(t *testing.T, server *memory.MemoryServer) string {
	t.Helper()
	list, err := server.ListRecollections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, meta := range list {
		if meta.Name == memory.MainName {
			return meta.ID
		}
	}
	t.Fatal("Main library not created")
	return ""
}

func memoryReadJSON(t *testing.T, path string, value any) {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, value); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryServerMinimalFilesAndMainProtection(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	main := mainMemoryID(t, server)
	root := filepath.Join(data, "memory")
	var index map[string]string
	memoryReadJSON(t, filepath.Join(root, "index.json"), &index)
	if !reflect.DeepEqual(index, map[string]string{main: "Main"}) {
		t.Fatalf("index is not a simple ID/name map: %+v", index)
	}
	var raw map[string]json.RawMessage
	memoryReadJSON(t, filepath.Join(root, "Main", "meta.json"), &raw)
	if len(raw) != 5 || raw["date"] == nil || raw["scope"] == nil {
		t.Fatalf("meta contains missing or unnecessary fields: %+v", raw)
	}
	var recollection memory.Recollection
	memoryReadJSON(t, filepath.Join(root, "Main", "data.json"), &recollection)
	if recollection.Name != "Main" || recollection.Data == nil || len(recollection.Data) != 0 {
		t.Fatalf("data is not a complete Recollection: %+v", recollection)
	}
	if err := server.Init(ctx); err != nil || mainMemoryID(t, server) != main {
		t.Fatalf("reinitialization replaced Main: %v", err)
	}
	for _, name := range []string{"Main", "main", "MAIN", "../outside", "CON", "NUL.txt", "LPT¹", "bad/name", "trailing.", "bad ", ".archive"} {
		if _, err := server.CreateRecollection(ctx, name, "", memory.ScopeGlobal, ""); err == nil {
			t.Errorf("invalid or reserved name accepted: %q", name)
		}
	}
	if err := server.UpdateRecollection(ctx, main, "renamed", ""); err == nil {
		t.Fatal("Main rename allowed")
	}
	if err := server.DeleteRecollection(ctx, main); err == nil {
		t.Fatal("Main deletion allowed")
	}
}

func TestMemoryServerScopesWorkspacesAndArbitrarySelection(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	workspaces := []chat.WorkSpace{{ID: id.MustNew(id.Workspace), Name: "项目"}, {ID: id.MustNew(id.Workspace), Name: "项目"}, {ID: id.MustNew(id.Workspace), Name: "Main"}}
	stored, _ := json.Marshal(map[string]any{"workspaces": workspaces})
	if err := os.WriteFile(filepath.Join(data, "workspaces.json"), stored, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := server.EnsureWorkspace(ctx, workspaces[0].ID, "项目已改名")
	if err != nil || first.Name != "项目" || first.Scope != memory.ScopeWorkspace {
		t.Fatalf("workspace identity was tied to display name: %+v / %v", first, err)
	}
	second, err := server.EnsureWorkspace(ctx, workspaces[1].ID, "项目")
	if err != nil || first.ID == second.ID || strings.EqualFold(first.Name, second.Name) {
		t.Fatalf("same-name workspaces share a directory: %+v / %v", second, err)
	}
	if err := server.SyncWorkspaces(ctx, workspaces); err != nil {
		t.Fatal(err)
	}
	list, err := server.ListRecollections(ctx)
	if err != nil || len(list) != 4 {
		t.Fatalf("workspace sync duplicates libraries: %+v / %v", list, err)
	}
	global := mainMemoryID(t, server)
	for _, library := range []string{global, first.ID, second.ID} {
		if _, err := server.AddMemory(ctx, library, "Go 项目经验", "编程"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := server.Search(ctx, []string{second.ID, first.ID, second.ID}, "go")
	if err != nil || len(result) != 2 || len(result[first.ID]) != 1 || len(result[second.ID]) != 1 || result[global] != nil {
		t.Fatalf("explicit cross-workspace selection was not respected: %+v / %v", result, err)
	}
	result, err = server.Search(ctx, nil, "Go")
	if err != nil || len(result) != 0 {
		t.Fatalf("disabled Main or empty selection was ignored: %+v / %v", result, err)
	}
	if _, err := server.Search(ctx, []string{"missing"}); err == nil {
		t.Fatal("missing enabled library silently accepted")
	}
	if _, err := server.CreateRecollection(ctx, "错误", "", memory.ScopeGlobal, workspaces[0].ID); err == nil {
		t.Fatal("global library accepted a workspace binding")
	}
}

func TestMemoryServerHistoryDeepCopiesAndSingleDate(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	main := mainMemoryID(t, server)
	key, err := server.AddMemory(ctx, main, "旧内容", "旧标签")
	if err != nil {
		t.Fatal(err)
	}
	copy, _ := server.GetRecollection(ctx, main)
	old := copy.Data[key]
	meta, _ := server.GetMeta(ctx, main)
	if meta.Date != old.MemoryDate {
		t.Fatal("one modification uses different metadata and memory timestamps")
	}
	if _, err := time.Parse(time.RFC3339Nano, old.MemoryDate); err != nil {
		t.Fatal(err)
	}
	(*copy.Data[key].Tag)[0] = "外部修改"
	fresh, _ := server.GetRecollection(ctx, main)
	if (*fresh.Data[key].Tag)[0] != "旧标签" {
		t.Fatal("returned tags modify stored memory")
	}
	if err := server.UpdateMemory(ctx, main, key, "新内容", "新标签"); err != nil {
		t.Fatal(err)
	}
	history, err := server.History(ctx, main, key)
	if err != nil || len(history) != 1 || history[0].Data != "旧内容" || (*history[0].Tag)[0] != "旧标签" || history[0].MemoryDate != old.MemoryDate {
		t.Fatalf("old snapshot was not preserved: %+v / %v", history, err)
	}
	(*history[0].Tag)[0] = "污染"
	if err := server.UpdateMemory(ctx, main, key, "新内容", "新标签"); err != nil {
		t.Fatal(err)
	}
	history, _ = server.History(ctx, main, key)
	if len(history) != 1 || (*history[0].Tag)[0] != "旧标签" {
		t.Fatal("no-op update adds history or history pointer leaked")
	}
	if err := server.RestoreMemory(ctx, main, key, 0); err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateMemory(ctx, main, key, "第三版", "tag"); err != nil {
		t.Fatal(err)
	}
	history, _ = server.History(ctx, main, key)
	if len(history) != 3 || history[0].Data != history[2].Data {
		t.Fatalf("duplicate snapshots should remain allowed: %+v", history)
	}
	if err := server.DeleteMemory(ctx, main, key); err != nil {
		t.Fatal(err)
	}
	fresh, _ = server.GetRecollection(ctx, main)
	if _, exists := fresh.Data[key]; exists {
		t.Fatal("deleted memory remains current")
	}
	history, _ = server.History(ctx, main, key)
	if len(history) != 4 || history[3].Data != "第三版" {
		t.Fatal("deletion lost the old memory")
	}
	var stored map[string][]memory.Memory
	memoryReadJSON(t, filepath.Join(data, "memory", "Main", "history.json"), &stored)
	if len(stored[key]) != 4 {
		t.Fatal("history is not a plain ID-to-snapshots map")
	}
}

func TestMemoryServerRenameArchiveAndReload(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	meta, err := server.CreateRecollection(ctx, "原名", "介绍", memory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := server.AddMemory(ctx, meta.ID, "正文", "tag")
	if err := server.UpdateMemory(ctx, meta.ID, key, "正文2", "tag2"); err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateRecollection(ctx, meta.ID, "新名", "新介绍"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "memory", "原名")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("old directory remains after rename")
	}
	reloaded := memory.NewMemoryServer(data, &desktop.DesktopFileSystem{}, nil)
	if err := reloaded.Init(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := reloaded.GetRecollection(ctx, meta.ID)
	if err != nil || r.Name != "新名" || r.Description != "新介绍" || r.Data[key].Data != "正文2" {
		t.Fatalf("rename or data did not persist: %+v / %v", r, err)
	}
	history, _ := reloaded.History(ctx, meta.ID, key)
	if len(history) != 1 || history[0].Data != "正文" {
		t.Fatal("rename dropped history")
	}
	if err := reloaded.DeleteRecollection(ctx, meta.ID); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(data, "memory", ".archive", meta.ID)
	if _, err := os.Stat(filepath.Join(archive, "history.json")); err != nil {
		t.Fatal("library deletion did not archive history")
	}
	if _, err := reloaded.CreateRecollection(ctx, "新名", "", memory.ScopeGlobal, ""); err != nil {
		t.Fatalf("archived directory did not release its name: %v", err)
	}
}

type failedMemoryMove struct {
	platform.FilePlatform
	target string
	fail   bool
}

func (f *failedMemoryMove) MoveFile(from, to string) error {
	if f.fail && to == f.target {
		f.fail = false
		return errors.New("simulated write interruption")
	}
	return f.FilePlatform.MoveFile(from, to)
}

func TestMemoryServerRecoversInterruptedWriteWithoutDuplicateHistory(t *testing.T) {
	for _, filename := range []string{"data.json", "history.json", "meta.json"} {
		t.Run(filename, func(t *testing.T) {
			ctx := context.Background()
			data := t.TempDir()
			fileSystem := &failedMemoryMove{FilePlatform: &desktop.DesktopFileSystem{}, target: filepath.Join(data, "memory", "Main", filename)}
			server := memory.NewMemoryServer(data, fileSystem, nil)
			if err := server.Init(ctx); err != nil {
				t.Fatal(err)
			}
			main := mainMemoryID(t, server)
			key, _ := server.AddMemory(ctx, main, "before", "old")
			fileSystem.fail = true
			if err := server.UpdateMemory(ctx, main, key, "after", "new"); err == nil {
				t.Fatal("write interruption did not return an error")
			}
			reloaded := memory.NewMemoryServer(data, fileSystem, nil)
			if err := reloaded.Init(ctx); err != nil {
				t.Fatalf("pending write could not recover: %v", err)
			}
			r, _ := reloaded.GetRecollection(ctx, main)
			if r.Data[key].Data != "after" {
				t.Fatal("recovery left an old current value")
			}
			history, err := reloaded.History(ctx, main, key)
			if err != nil || len(history) != 1 || history[0].Data != "before" {
				t.Fatalf("replay duplicated or lost history: %+v / %v", history, err)
			}
			if err := reloaded.Init(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(data, "memory", ".pending.json")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("pending write remains after success")
			}
		})
	}
}

func TestMemoryServerDetectsCorruptionAndLoadsHistoryOnDemand(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	main := mainMemoryID(t, server)
	key, _ := server.AddMemory(ctx, main, "current", "tag")
	root := filepath.Join(data, "memory")
	if err := os.WriteFile(filepath.Join(root, "Main", "history.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.Init(ctx); err != nil {
		t.Fatalf("normal startup unnecessarily parses history: %v", err)
	}
	if _, err := server.Search(ctx, []string{main}, "current"); err != nil {
		t.Fatalf("current search reads old history: %v", err)
	}
	if _, err := server.History(ctx, main, key); err == nil {
		t.Fatal("history query ignored corrupted history")
	}
	if err := server.UpdateMemory(ctx, main, key, "replacement", "tag"); err == nil {
		t.Fatal("update overwrote corrupted history")
	}
	current, _ := server.GetRecollection(ctx, main)
	if current.Data[key].Data != "current" {
		t.Fatal("failed update modified current memory")
	}
	if err := os.Remove(filepath.Join(root, "index.json")); err != nil {
		t.Fatal(err)
	}
	if err := server.Init(ctx); err != nil {
		t.Fatalf("missing index was not rebuilt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.Init(ctx); err == nil {
		t.Fatal("incorrect existing index was silently overwritten")
	}
	if err := server.RebuildIndex(ctx); err != nil {
		t.Fatalf("explicit index rebuild failed: %v", err)
	}
	if err := server.Init(ctx); err != nil {
		t.Fatalf("rebuilt index cannot load: %v", err)
	}
}

func TestMemoryServerSaveWholeRecollectionAndRecoverRename(t *testing.T) {
	server, data := testMemoryServer(t)
	ctx := context.Background()
	main := mainMemoryID(t, server)
	first, _ := server.AddMemory(ctx, main, "first", "a")
	second, _ := server.AddMemory(ctx, main, "second", "b")
	r, err := server.GetRecollection(ctx, main)
	if err != nil {
		t.Fatal(err)
	}
	delete(r.Data, first)
	changed := r.Data[second]
	changed.Data = "second edited"
	changed.MemoryDate = "caller-supplied-invalid-date"
	r.Data[second] = changed
	if err := server.SaveRecollection(ctx, main, *r); err != nil {
		t.Fatal(err)
	}
	history, _ := server.History(ctx, main, first)
	if len(history) != 1 || history[0].Data != "first" {
		t.Fatal("whole-library save did not archive removed unit")
	}
	current, _ := server.GetRecollection(ctx, main)
	if current.Data[second].MemoryDate == changed.MemoryDate {
		t.Fatal("caller overwrote the managed timestamp")
	}
	fileSystem := &failedMemoryMove{FilePlatform: &desktop.DesktopFileSystem{}}
	renameServer := memory.NewMemoryServer(data, fileSystem, nil)
	if err := renameServer.Init(ctx); err != nil {
		t.Fatal(err)
	}
	meta, err := renameServer.CreateRecollection(ctx, "Before", "", memory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	fileSystem.target, fileSystem.fail = filepath.Join(data, "memory", "After", "meta.json"), true
	if err := renameServer.UpdateRecollection(ctx, meta.ID, "After", "updated"); err == nil {
		t.Fatal("rename interruption did not return error")
	}
	reloaded := memory.NewMemoryServer(data, fileSystem, nil)
	if err := reloaded.Init(ctx); err != nil {
		t.Fatal(err)
	}
	meta, err = reloaded.GetMeta(ctx, meta.ID)
	if err != nil || meta.Name != "After" || meta.Description != "updated" {
		t.Fatalf("rename recovery failed: %+v / %v", meta, err)
	}
}

func TestMemoryServerConcurrentMutationsAndCancellation(t *testing.T) {
	server, _ := testMemoryServer(t)
	ctx := context.Background()
	main := mainMemoryID(t, server)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := server.AddMemory(ctx, main, "memory", "tag"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	r, err := server.GetRecollection(ctx, main)
	if err != nil || len(r.Data) != 16 {
		t.Fatalf("concurrent updates were lost: %+v / %v", r, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := server.AddMemory(cancelled, main, "cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write was accepted: %v", err)
	}
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
