package Tests

import (
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	backend "github.com/caliniya/caf/core/Backend"
	platform "github.com/caliniya/caf/platform"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
	"github.com/gorilla/websocket"
)

func useQQAdapterCore(t *testing.T) *backend.Core {
	t.Helper()
	instance := &backend.Core{
		Paths:               &platform.Paths{Data: t.TempDir()},
		FS:                  &desktopBackend.DesktopFileSystem{},
		QQAdapterListenHost: "127.0.0.1",
	}
	t.Cleanup(instance.CloseQQAdapters)
	return instance
}

func waitQQAdapter(t *testing.T, instance *backend.Core, id, state, account string) *backend.QQAdapter {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		adapter, err := instance.GetQQAdapter(id)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.State == state && adapter.QQ == account {
			return adapter
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("adapter did not reach %s / QQ %s", state, account)
	return nil
}

func TestQQAdaptersPersistAndValidate(t *testing.T) {
	instance := useQQAdapterCore(t)
	for _, input := range [][3]string{
		{"", "token", ""}, {"name", "", ""}, {"name", "token", "abc"},
		{"name", "token", "0"}, {"name", "token", "-1"}, {"name", "token", "9223372036854775808"},
	} {
		if _, err := instance.CreateQQAdapter(input[0], input[1], input[2]); err == nil {
			t.Fatalf("accepted invalid adapter: %q", input)
		}
	}
	preview, err := instance.GetQQAdapterAddress()
	if err != nil {
		t.Fatal(err)
	}
	first, err := instance.CreateQQAdapter(" First ", " first-token ", " 0010001 ")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "First" || first.QQ != "10001" || first.State != "stopped" || first.Address != preview || first.Token != "" {
		t.Fatalf("unexpected new adapter: %+v", first)
	}
	second, err := instance.CreateQQAdapter("Second", "second-token", "")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Address == first.Address || second.QQ != "" {
		t.Fatalf("adapters must have separate endpoints: %+v / %+v", first, second)
	}
	reloaded := &backend.Core{Paths: instance.Paths, FS: instance.FS}
	adapters, err := reloaded.ListQQAdapters()
	if err != nil || len(adapters) != 2 {
		t.Fatalf("failed to restore adapter list: %+v / %v", adapters, err)
	}
	details, err := reloaded.GetQQAdapter(first.ID)
	if err != nil || details.Token != "first-token" {
		t.Fatalf("edit details did not restore token: %+v / %v", details, err)
	}
	for _, adapter := range adapters {
		if adapter.Token != "" || adapter.State != "stopped" {
			t.Fatalf("list exposed a token or started an adapter: %+v", adapter)
		}
	}
	updated, err := instance.UpdateQQAdapter(first.ID, "Renamed", "new-token", "20002")
	if err != nil || updated.Name != "Renamed" || updated.QQ != "20002" || updated.Address != first.Address {
		t.Fatalf("failed to edit stopped adapter: %+v / %v", updated, err)
	}
	if err := instance.DeleteQQAdapter(second.ID); err != nil {
		t.Fatal(err)
	}
	reloaded = &backend.Core{Paths: instance.Paths, FS: instance.FS}
	adapters, err = reloaded.ListQQAdapters()
	if err != nil || len(adapters) != 1 || adapters[0].Name != "Renamed" {
		t.Fatalf("edits/deletion did not persist: %+v / %v", adapters, err)
	}
}

func TestQQAdaptersRealLifecycleAndDetectedAccount(t *testing.T) {
	instance := useQQAdapterCore(t)
	adapter, err := instance.CreateQQAdapter("Bot", "token", "")
	if err != nil {
		t.Fatal(err)
	}
	started, err := instance.StartQQAdapter(adapter.ID)
	if err != nil || started.State != "disconnected" {
		t.Fatalf("startup did not wait for connection: %+v / %v", started, err)
	}
	if _, err := instance.StartQQAdapter(adapter.ID); err != nil {
		t.Fatalf("repeated start should be harmless: %v", err)
	}
	headers := http.Header{"X-Self-ID": {"10001"}, "Authorization": {"Bearer wrong-token"}}
	connection, response, err := websocket.DefaultDialer.Dial(adapter.Address, headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatal("adapter accepted incorrect token")
	}
	connection = connectReverseWS(t, adapter.Address, "10001", "token")
	waitQQAdapter(t, instance, adapter.ID, "connected", "10001")
	if renamed, err := instance.UpdateQQAdapter(adapter.ID, "Live rename", "token", ""); err != nil || renamed.State != "connected" {
		t.Fatalf("renaming should preserve active connection: %+v / %v", renamed, err)
	}
	if _, err := instance.UpdateQQAdapter(adapter.ID, "Bot", "changed-token", ""); err == nil {
		t.Fatal("changed token while adapter was running")
	}
	if _, err := instance.UpdateQQAdapter(adapter.ID, "Bot", "token", "20002"); err == nil {
		t.Fatal("changed QQ account while adapter was running")
	}
	// 等待事件消费者将自动识别的 QQ 号写入配置。
	deadline := time.Now().Add(3 * time.Second)
	for {
		reloaded := &backend.Core{Paths: instance.Paths, FS: instance.FS}
		details, err := reloaded.GetQQAdapter(adapter.ID)
		if err == nil && details.QQ == "10001" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("detected QQ account did not persist")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = connection.Close()
	waitQQAdapter(t, instance, adapter.ID, "disconnected", "10001")
	connection = connectReverseWS(t, adapter.Address, "10001", "token")
	waitQQAdapter(t, instance, adapter.ID, "connected", "10001")
	stopped, err := instance.StopQQAdapter(adapter.ID)
	if err != nil || stopped.State != "stopped" || stopped.QQ != "10001" {
		t.Fatalf("stop lost status/account: %+v / %v", stopped, err)
	}
	if _, err := instance.StartQQAdapter(adapter.ID); err != nil {
		t.Fatalf("could not restart adapter: %v", err)
	}
	connection = connectReverseWS(t, adapter.Address, "10001", "token")
	waitQQAdapter(t, instance, adapter.ID, "connected", "10001")
	if err := instance.DeleteQQAdapter(adapter.ID); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("delete did not close active WebSocket")
	}
	endpoint, _ := url.Parse(adapter.Address)
	listener, err := net.Listen("tcp", endpoint.Host)
	if err != nil {
		t.Fatalf("delete did not release listener: %v", err)
	}
	_ = listener.Close()
	if _, err := instance.GetQQAdapter(adapter.ID); err == nil {
		t.Fatal("deleted adapter still exists")
	}
}

type failingQQAdapterFS struct {
	platform.FilePlatform
}

func (failingQQAdapterFS) WriteFile(string, []byte, fs.FileMode) error {
	return errors.New("test write failure")
}

func TestQQAdaptersFailuresPreserveConfigurationAndRuntime(t *testing.T) {
	instance := useQQAdapterCore(t)
	adapter, err := instance.CreateQQAdapter("Bot", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := url.Parse(adapter.Address)
	occupied, err := net.Listen("tcp", endpoint.Host)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.StartQQAdapter(adapter.ID); err == nil {
		t.Fatal("startup succeeded on an occupied port")
	}
	waitQQAdapter(t, instance, adapter.ID, "stopped", "10001")
	_ = occupied.Close()
	if _, err := instance.StartQQAdapter(adapter.ID); err != nil {
		t.Fatal(err)
	}
	originalFS := instance.FS
	instance.FS = failingQQAdapterFS{originalFS}
	if _, err := instance.CreateQQAdapter("Second", "token", ""); err == nil {
		t.Fatal("create succeeded despite failed persistence")
	}
	if _, err := instance.UpdateQQAdapter(adapter.ID, "Changed", "token", "10001"); err == nil {
		t.Fatal("rename succeeded despite failed persistence")
	}
	if err := instance.DeleteQQAdapter(adapter.ID); err == nil {
		t.Fatal("delete succeeded despite failed persistence")
	}
	instance.FS = originalFS
	adapters, err := instance.ListQQAdapters()
	if err != nil || len(adapters) != 1 || adapters[0].Name != "Bot" || adapters[0].State != "disconnected" {
		t.Fatalf("failed changes altered adapter: %+v / %v", adapters, err)
	}
	connection := connectReverseWS(t, adapter.Address, "10001", "token")
	waitQQAdapter(t, instance, adapter.ID, "connected", "10001")
	_ = connection.Close()
	instance.CloseQQAdapters()
	waitQQAdapter(t, instance, adapter.ID, "stopped", "10001")
	if err := os.WriteFile(filepath.Join(instance.Paths.Data, "qq", "adapters.json"), []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded := &backend.Core{Paths: instance.Paths, FS: instance.FS}
	if _, err := reloaded.ListQQAdapters(); err == nil {
		t.Fatal("silently accepted corrupted adapter configuration")
	}
}
