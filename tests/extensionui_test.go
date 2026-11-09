package Tests

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	core "github.com/caliniya/caf/core"
	qq "github.com/caliniya/caf/core/Extension/QQ"
	platform "github.com/caliniya/caf/platform"
)

func TestExtensionUIDescribesFormsAndInformation(t *testing.T) {
	instance := useQQAdapterCore(t)
	types := instance.ListExtensionTypes()
	if len(types) != 1 || types[0].Type != qq.ExtensionType || types[0].Name != "Onebot11QQ" {
		t.Fatalf("wrong extension type: %+v", types)
	}
	fields := types[0].Parameters
	if len(fields) != 4 || fields[0].Key != "name" || !fields[0].Required || fields[1].Key != "token" || fields[1].Kind != "secret" || fields[2].Required || fields[3].Key != "port" || fields[3].Minimum != 1 || fields[3].Maximum != 65535 {
		t.Fatalf("creation metadata incomplete: %+v", fields)
	}
	info := types[0].Information
	if len(info) != 2 || info[0].Key != "status" || !info[0].InList || info[1].Key != "address" || info[1].InList {
		t.Fatalf("wrong list/detail visibility: %+v", info)
	}
	types[0].Parameters[0].Label = "changed"
	if instance.ListExtensionTypes()[0].Parameters[0].Label != "名称" {
		t.Fatal("descriptor is not a snapshot")
	}
	client, address := useQQExtension(t, qq.ReverseWSConfig{})
	values := client.UIValues()
	if values["status"] != "未连接" || values["address"] != address {
		t.Fatalf("wrong runtime UI values: %+v", values)
	}
	values["status"] = "changed"
	connectReverseWS(t, address, "10001", "")
	nextExtensionEvent(t, client, "qq.status")
	if client.UIValues()["status"] != "已连接" {
		t.Fatal("runtime UI values did not follow connection")
	}
}

func TestExtensionUICreatesPortsListsDisabledAndEditsBindings(t *testing.T) {
	instance := useQQAdapterCore(t)
	preview, err := instance.GetQQAdapterAddress()
	if err != nil {
		t.Fatal(err)
	}
	portText := strings.Split(strings.TrimPrefix(preview, "ws://127.0.0.1:"), "/")[0]
	parameters := map[string]string{"name": "助手", "token": "token", "selfId": "10001", "port": portText}
	created, err := instance.CreateExtension(qq.ExtensionType, parameters)
	if err != nil || !strings.Contains(created.Values["address"], ":"+portText+"/") {
		t.Fatalf("explicit port not used: %+v / %v", created, err)
	}
	for _, invalid := range []string{"0", "65536", "bad", "", "1.5"} {
		bad := map[string]string{"name": "other", "token": "token", "port": invalid}
		if _, err := instance.CreateExtension(qq.ExtensionType, bad); err == nil {
			t.Fatalf("invalid port accepted %q", invalid)
		}
	}
	if _, err := instance.CreateExtension("unknown", parameters); err == nil {
		t.Fatal("accepted unknown type")
	}
	if _, err := instance.CreateExtension(qq.ExtensionType, parameters); err == nil {
		t.Fatal("accepted duplicate port")
	}
	owner, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetExtensionClientQQ(owner.ID, created.ID, parameters["name"], false); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetClientExtensionEnabled(owner.ID, created.ID, true); err != nil {
		t.Fatal(err)
	}
	options, err := instance.ListExtensions()
	if err != nil || len(options) != 1 || !options[0].Enabled || options[0].ClientID != owner.ID || options[0].Values["status"] != "未连接" {
		t.Fatalf("enabled option missing: %+v / %v", options, err)
	}
	data, _ := json.Marshal(options)
	if strings.Contains(string(data), `"token"`) {
		t.Fatal("list exposed token")
	}
	parameters["name"] = "新助手"
	if err := instance.UpdateExtensionParameters(created.ID, parameters); err != nil {
		t.Fatal(err)
	}
	owner, err = instance.GetExtensionClient(owner.ID)
	if err != nil || owner.Extensions[0].Name != "新助手" || owner.Extensions[0].Type != qq.ExtensionType {
		t.Fatalf("rename not synced with command binding: %+v / %v", owner, err)
	}
	parameters["port"] = "65001"
	if err := instance.UpdateExtensionParameters(created.ID, parameters); err == nil {
		t.Fatal("changed port while running")
	}
	if _, err := instance.SetClientExtensionEnabled(owner.ID, created.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := instance.UpdateExtensionParameters(created.ID, parameters); err != nil {
		t.Fatal(err)
	}
	stored, err := instance.GetExtensionParameters(created.ID)
	if err != nil || stored["port"] != "65001" || stored["name"] != "新助手" {
		t.Fatalf("parameter edit not persisted: %+v / %v", stored, err)
	}
	options, err = instance.ListExtensions()
	if err != nil || len(options) != 1 || options[0].Enabled || options[0].Values["status"] != "未启动" || !strings.Contains(options[0].Values["address"], ":65001/") {
		t.Fatalf("disabled instance disappeared or stale: %+v / %v", options, err)
	}
	// 旧的绑定类型在读取时迁移，保持已有实例和命令名称。
	file := filepath.Join(instance.Paths.Data, "ext", "clients", owner.ID, "client.json")
	data, err = instance.FS.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), `"type": "onebot11qq"`, `"type": "qq"`))
	if err := instance.FS.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	owner, err = instance.GetExtensionClient(owner.ID)
	if err != nil || owner.Extensions[0].Type != qq.ExtensionType {
		t.Fatalf("old type not migrated: %+v / %v", owner, err)
	}
}

func TestClientExtensionsAreIndependent(t *testing.T) {
	instance := useQQAdapterCore(t)
	firstClient, err := instance.CreateExtensionClient("First", "first")
	if err != nil {
		t.Fatal(err)
	}
	secondClient, err := instance.CreateExtensionClient("Second", "second")
	if err != nil {
		t.Fatal(err)
	}
	create := func(clientID, token string) *core.ExtensionOption {
		t.Helper()
		preview, err := instance.GetQQAdapterAddress()
		if err != nil {
			t.Fatal(err)
		}
		port := strings.Split(strings.TrimPrefix(preview, "ws://127.0.0.1:"), "/")[0]
		option, err := instance.CreateClientExtension(clientID, qq.ExtensionType, map[string]string{"name": "助手", "token": token, "port": port})
		if err != nil {
			t.Fatal(err)
		}
		if option.ClientID != clientID || option.Enabled || option.Running {
			t.Fatalf("new extension not owned/stopped: %+v", option)
		}
		return option
	}
	first := create(firstClient.ID, "first-token")
	second := create(secondClient.ID, "second-token")
	if first.ID == second.ID || first.Values["address"] == second.Values["address"] {
		t.Fatal("clients reused one extension instance")
	}
	for _, pair := range []struct{ clientID, instanceID string }{{firstClient.ID, first.ID}, {secondClient.ID, second.ID}} {
		options, err := instance.ListClientExtensions(pair.clientID)
		if err != nil || len(options) != 1 || options[0].ID != pair.instanceID {
			t.Fatalf("client sees other extensions: %+v / %v", options, err)
		}
	}
	if _, err := instance.CreateClientExtension(firstClient.ID, qq.ExtensionType, map[string]string{"name": "助手", "token": "token", "port": "65001"}); err == nil {
		t.Fatal("same-client name collision accepted")
	}
	if _, err := instance.GetClientExtensionParameters(secondClient.ID, first.ID); err == nil {
		t.Fatal("cross-client parameter read accepted")
	}
	parameters, err := instance.GetClientExtensionParameters(firstClient.ID, first.ID)
	if err != nil || parameters["token"] != "first-token" {
		t.Fatalf("wrong client's token: %+v / %v", parameters, err)
	}
	parameters["name"] = "renamed"
	if err := instance.UpdateClientExtensionParameters(secondClient.ID, first.ID, parameters); err == nil {
		t.Fatal("cross-client edit accepted")
	}
	if _, err := instance.SetClientExtensionEnabled(secondClient.ID, first.ID, true); err == nil {
		t.Fatal("cross-client enabling accepted")
	}
	if err := instance.DeleteClientExtension(secondClient.ID, first.ID); err == nil {
		t.Fatal("cross-client delete accepted")
	}
	if err := instance.UpdateClientExtensionParameters(firstClient.ID, first.ID, parameters); err != nil {
		t.Fatal(err)
	}
	secondParameters, err := instance.GetClientExtensionParameters(secondClient.ID, second.ID)
	if err != nil || secondParameters["name"] != "助手" || secondParameters["token"] != "second-token" {
		t.Fatalf("edit changed other client: %+v / %v", secondParameters, err)
	}
	if _, err := instance.SetClientExtensionEnabled(firstClient.ID, first.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetClientExtensionEnabled(secondClient.ID, second.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := instance.DeleteClientExtension(firstClient.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	options, err := instance.ListClientExtensions(firstClient.ID)
	if err != nil || len(options) != 0 {
		t.Fatalf("deleted extension still listed: %+v / %v", options, err)
	}
	if _, err := instance.GetQQAdapter(first.ID); err == nil {
		t.Fatal("delete left global extension instance")
	}
	options, err = instance.ListClientExtensions(secondClient.ID)
	if err != nil || len(options) != 1 || !options[0].Enabled || !options[0].Running {
		t.Fatalf("deleting affected other client: %+v / %v", options, err)
	}
	reloaded := &core.Core{Paths: instance.Paths, FS: instance.FS}
	options, err = reloaded.ListClientExtensions(secondClient.ID)
	if err != nil || len(options) != 1 || options[0].ID != second.ID {
		t.Fatalf("ownership not persisted: %+v / %v", options, err)
	}
}

func TestClientExtensionCreationRollsBackFailedOwnershipSave(t *testing.T) {
	instance := useQQAdapterCore(t)
	owner, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := instance.GetQQAdapterAddress()
	if err != nil {
		t.Fatal(err)
	}
	port := strings.Split(strings.TrimPrefix(preview, "ws://127.0.0.1:"), "/")[0]
	original := instance.FS
	instance.FS = failingClientMetadataFS{original}
	_, err = instance.CreateClientExtension(owner.ID, qq.ExtensionType, map[string]string{"name": "助手", "token": "token", "port": port})
	instance.FS = original
	if err == nil {
		t.Fatal("created extension without persisted owner")
	}
	options, err := instance.ListQQAdapters()
	if err != nil || len(options) != 0 {
		t.Fatalf("failed creation left unowned instance: %+v / %v", options, err)
	}
	client, err := instance.GetExtensionClient(owner.ID)
	if err != nil || len(client.Extensions) != 0 {
		t.Fatalf("failed creation changed client: %+v / %v", client, err)
	}
	if _, err := instance.CreateClientExtension(owner.ID, qq.ExtensionType, map[string]string{"name": "助手", "token": "token", "port": port}); err != nil {
		t.Fatalf("failed creation reserved port/name: %v", err)
	}
}

type failingClientMetadataFS struct{ platform.FilePlatform }

func (f failingClientMetadataFS) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if strings.HasSuffix(path, "client.json.tmp") {
		return errors.New("test client metadata save failure")
	}
	return f.FilePlatform.WriteFile(path, data, mode)
}

func TestExtensionUIBoundEditRollsBackFailedClientSave(t *testing.T) {
	instance := useQQAdapterCore(t)
	client, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := instance.CreateQQAdapter("助手", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.SetExtensionClientQQ(client.ID, adapter.ID, "助手", false); err != nil {
		t.Fatal(err)
	}
	parameters, err := instance.GetExtensionParameters(adapter.ID)
	if err != nil {
		t.Fatal(err)
	}
	parameters["name"] = "Changed"
	original := instance.FS
	instance.FS = failingClientMetadataFS{original}
	err = instance.UpdateExtensionParameters(adapter.ID, parameters)
	instance.FS = original
	if err == nil {
		t.Fatal("edit succeeded despite failed client save")
	}
	stored, err := instance.GetQQAdapter(adapter.ID)
	if err != nil || stored.Name != "助手" {
		t.Fatalf("adapter name not rolled back: %+v / %v", stored, err)
	}
	client, err = instance.GetExtensionClient(client.ID)
	if err != nil || client.Extensions[0].Name != "助手" {
		t.Fatalf("client binding changed: %+v / %v", client, err)
	}
}

func TestExtensionUIStartsOnConfiguredPort(t *testing.T) {
	instance := useQQAdapterCore(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	adapter, err := instance.CreateExtension(qq.ExtensionType, map[string]string{"name": "bot", "token": "token", "port": strconv.Itoa(port)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.StartQQAdapter(adapter.ID); err != nil {
		t.Fatal(err)
	}
	connectReverseWS(t, adapter.Values["address"], "10001", "token")
	waitQQAdapter(t, instance, adapter.ID, "connected", "10001")
}
