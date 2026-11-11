package Tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backend "github.com/caliniya/caf/core/Backend"
	platform "github.com/caliniya/caf/platform"
)

func TestExtensionClientsUseInternalIDFolders(t *testing.T) {
	instance := useQQAdapterCore(t)
	for _, id := range []string{"", "..", "../outside", `a\b`, "a/b", "a:b", "a.b", "CON", "com1", strings.Repeat("a", 65)} {
		if _, err := instance.CreateExtensionClient("name", id); err == nil {
			t.Fatalf("accepted invalid ID %q", id)
		}
	}
	if _, err := instance.CreateExtensionClient("", "client"); err == nil {
		t.Fatal("accepted empty name")
	}
	client, err := instance.CreateExtensionClient(" First ", "first-client")
	if err != nil {
		t.Fatal(err)
	}
	if client.ID != "first-client" || client.Name != "First" || len(client.Extensions) != 0 {
		t.Fatalf("invalid client: %+v", client)
	}
	file := filepath.Join(instance.Paths.Data, "ext", "clients", client.ID, "client.json")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("ID folder not created: %v", err)
	}
	if _, err := instance.CreateExtensionClient("duplicate", "FIRST-CLIENT"); err == nil {
		t.Fatal("accepted case-insensitive duplicate ID")
	}
	reloaded := &backend.Core{Paths: instance.Paths, FS: instance.FS}
	list, err := reloaded.ListExtensionClients()
	if err != nil || len(list) != 1 || list[0].ID != client.ID {
		t.Fatalf("client did not persist: %+v / %v", list, err)
	}
	if _, err := instance.RenameExtensionClient(client.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	oldData := filepath.Join(filepath.Dir(file), "history.txt")
	if err := os.WriteFile(oldData, []byte("old client data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := instance.DeleteExtensionClient(client.ID); err != nil {
		t.Fatal(err)
	}
	list, err = instance.ListExtensionClients()
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted client remains: %+v / %v", list, err)
	}
	if _, err := os.Stat(filepath.Dir(file)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted client's ID folder was not released: %v", err)
	}
	archive := filepath.Join(instance.Paths.Data, "ext", "archive", "clients")
	entries, err := os.ReadDir(archive)
	if err != nil || len(entries) != 1 {
		t.Fatalf("client data not archived: %+v / %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(archive, entries[0].Name(), "history.txt"))
	if err != nil || string(data) != "old client data" {
		t.Fatalf("archive lost internal data: %q / %v", data, err)
	}
	fresh, err := reloaded.CreateExtensionClient("New", client.ID)
	if err != nil || fresh.Name != "New" || len(fresh.Extensions) != 0 {
		t.Fatalf("deleted ID not reusable after reload: %+v / %v", fresh, err)
	}
	if _, err := os.Stat(oldData); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new client inherited archived data")
	}
	if err := reloaded.DeleteExtensionClient(fresh.ID); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(archive)
	if err != nil || len(entries) != 2 {
		t.Fatalf("repeated deletion overwrote history: %+v / %v", entries, err)
	}
	if _, err := instance.CreateExtensionClient("Third", client.ID); err != nil {
		t.Fatalf("ID not reusable after repeated deletion: %v", err)
	}
	list, err = instance.ListExtensionClients()
	if err != nil || len(list) != 1 || list[0].Name != "Third" {
		t.Fatalf("archive leaked into client list: %+v / %v", list, err)
	}
}

func TestExtensionClientsReuseLegacyArchivedID(t *testing.T) {
	instance := useQQAdapterCore(t)
	client, err := instance.CreateExtensionClient("Legacy", "legacy")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(instance.Paths.Data, "ext", "clients", client.ID)
	if err := os.Rename(filepath.Join(path, "client.json"), filepath.Join(path, "archived.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "history.txt"), []byte("legacy history"), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := instance.CreateExtensionClient("Fresh", "LEGACY")
	if err != nil || fresh.ID != "LEGACY" {
		t.Fatalf("legacy deleted ID remained reserved: %+v / %v", fresh, err)
	}
	archive := filepath.Join(instance.Paths.Data, "ext", "archive", "clients")
	entries, err := os.ReadDir(archive)
	if err != nil || len(entries) != 1 {
		t.Fatalf("legacy archive missing: %+v / %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(archive, entries[0].Name(), "history.txt"))
	if err != nil || string(data) != "legacy history" {
		t.Fatalf("legacy data overwritten: %q / %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(archive, entries[0].Name(), "archived.json")); err != nil {
		t.Fatal("legacy configuration not preserved")
	}
	if _, err := os.Stat(filepath.Join(instance.Paths.Data, "ext", "clients", fresh.ID, "archived.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new client inherited legacy archive")
	}
}

type failingClientArchiveFS struct{ platform.FilePlatform }

func (failingClientArchiveFS) MoveFile(string, string) error {
	return errors.New("test archive move failed")
}

func TestExtensionClientsFailedArchivePreservesActiveClient(t *testing.T) {
	instance := useQQAdapterCore(t)
	client, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := instance.CreateQQAdapter("QQ", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetExtensionClientQQ(client.ID, adapter.ID, "助手", true); err != nil {
		t.Fatal(err)
	}
	original := instance.FS
	instance.FS = failingClientArchiveFS{original}
	err = instance.DeleteExtensionClient(client.ID)
	instance.FS = original
	if err == nil {
		t.Fatal("deletion succeeded despite failed archive")
	}
	if _, err := instance.GetExtensionClient(client.ID); err != nil {
		t.Fatalf("failed deletion lost client: %v", err)
	}
	waitQQAdapter(t, instance, adapter.ID, "disconnected", "10001")
	if _, err := instance.CreateExtensionClient("Duplicate", client.ID); err == nil {
		t.Fatal("failed deletion released active ID")
	}
	if err := instance.DeleteExtensionClient(client.ID); err != nil {
		t.Fatal(err)
	}
	waitQQAdapter(t, instance, adapter.ID, "stopped", "10001")
	fresh, err := instance.CreateExtensionClient("Fresh", client.ID)
	if err != nil || len(fresh.Extensions) != 0 {
		t.Fatalf("new client inherited old bindings: %+v / %v", fresh, err)
	}
}

func TestExtensionClientsMultipleNamedQQBindings(t *testing.T) {
	instance := useQQAdapterCore(t)
	client, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	other, err := instance.CreateExtensionClient("Other", "other")
	if err != nil {
		t.Fatal(err)
	}
	first, err := instance.CreateQQAdapter("First", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	second, err := instance.CreateQQAdapter("Second", "token", "20002")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "a b", "a/b", strings.Repeat("a", 33)} {
		if _, err := instance.SetExtensionClientQQ(client.ID, first.ID, name, true); err == nil {
			t.Fatalf("accepted invalid extension name %q", name)
		}
	}
	if _, err := instance.SetExtensionClientQQ(client.ID, first.ID, "助手", true); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetExtensionClientQQ(client.ID, second.ID, "助手", true); err == nil {
		t.Fatal("duplicate command name accepted")
	}
	client, err = instance.SetExtensionClientQQ(client.ID, second.ID, "另一个助手", true)
	if err != nil || len(client.Extensions) != 2 {
		t.Fatalf("multiple bindings not retained: %+v / %v", client, err)
	}
	if _, err := instance.SetExtensionClientQQ(other.ID, first.ID, "other", true); err == nil {
		t.Fatal("one adapter attached to multiple clients")
	}
	if err := instance.DeleteQQAdapter(first.ID); err == nil {
		t.Fatal("deleted bound adapter")
	}
	if _, err := instance.SetExtensionClientQQ(client.ID, first.ID, "助手", false); err != nil {
		t.Fatal(err)
	}
	waitQQAdapter(t, instance, first.ID, "stopped", "10001")
	waitQQAdapter(t, instance, second.ID, "disconnected", "20002")
	client, err = instance.RemoveExtensionClientQQ(client.ID, first.ID)
	if err != nil || len(client.Extensions) != 1 || client.Extensions[0].Name != "另一个助手" {
		t.Fatalf("removed wrong binding: %+v / %v", client, err)
	}
	if err := instance.DeleteQQAdapter(first.ID); err != nil {
		t.Fatal(err)
	}
	reloaded := &backend.Core{Paths: instance.Paths, FS: instance.FS}
	client, err = reloaded.GetExtensionClient(client.ID)
	if err != nil || len(client.Extensions) != 1 || !client.Extensions[0].Enabled {
		t.Fatalf("binding did not persist: %+v / %v", client, err)
	}
	instance.CloseQQAdapters()
	reloaded.QQ.ListenHost = instance.QQ.ListenHost
	reloaded.StartExtensionClients()
	t.Cleanup(reloaded.CloseQQAdapters)
	waitQQAdapter(t, reloaded, second.ID, "disconnected", "20002")
	if err := reloaded.DeleteExtensionClient(client.ID); err != nil {
		t.Fatal(err)
	}
	waitQQAdapter(t, reloaded, second.ID, "stopped", "20002")
}

func TestExtensionClientsFailedPersistenceDoesNotEnableQQ(t *testing.T) {
	instance := useQQAdapterCore(t)
	client, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := instance.CreateQQAdapter("QQ", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	original := instance.FS
	instance.FS = failingQQAdapterFS{original}
	if _, err := instance.SetExtensionClientQQ(client.ID, adapter.ID, "助手", true); err == nil {
		t.Fatal("enabled despite failed save")
	}
	if _, err := instance.CreateExtensionClient("Failure", "failure"); err == nil {
		t.Fatal("created despite failed save")
	}
	instance.FS = original
	waitQQAdapter(t, instance, adapter.ID, "stopped", "10001")
	client, err = instance.GetExtensionClient(client.ID)
	if err != nil || len(client.Extensions) != 0 {
		t.Fatalf("failed save changed bindings: %+v / %v", client, err)
	}
	if _, err := instance.CreateExtensionClient("Retry", "failure"); err != nil {
		t.Fatalf("failed creation reserved ID: %v", err)
	}
}
