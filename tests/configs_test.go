package Tests

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	platform "github.com/caliniya/caf/platform"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "MainConfig.json")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func useConfigFile(t *testing.T, file string) {
	t.Helper()
	previous := backend.Platform
	t.Cleanup(func() { backend.Platform = previous })
	backend.Platform = &backend.Core{
		Paths: &platform.Paths{ConfigFile: file},
		FS:    &desktopBackend.DesktopFileSystem{},
	}
}

func TestLoadConfigReadsFile(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://example.com","APIKey":"sk-test","Model":["m1","m2"]}]}`)
	useConfigFile(t, file)

	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	got := backend.Platform.Configs
	if got == nil || got.ALLConfig == nil || len(*got.ALLConfig) != 1 {
		t.Fatalf("unexpected configs: %+v", got)
	}
	entry := (*got.ALLConfig)[0]
	if entry.ServerAddress != "https://example.com" || entry.APIKey != "sk-test" || len(entry.Model) != 2 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestLoadConfigReportsMalformedJSON(t *testing.T) {
	file := writeConfig(t, "{")
	useConfigFile(t, file)
	previous := &backend.Configs{}
	backend.Platform.Configs = previous

	if _, err := backend.Platform.LoadConfigs(); err == nil {
		t.Fatal("malformed json accepted")
	}
	if backend.Platform.Configs != previous {
		t.Fatal("a failed reload replaced the current configuration")
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "{" {
		t.Fatalf("a malformed configuration was overwritten: %q, %v", data, err)
	}
}

func TestLoadConfigsCreatesMissingFile(t *testing.T) {
	for _, directory := range []string{"", filepath.Join("data", "config")} {
		t.Run(directory, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), directory, "MainConfig.json")
			useConfigFile(t, file)
			configs, err := backend.Platform.LoadConfigs()
			if err != nil {
				t.Fatal(err)
			}
			if configs == nil || configs.ALLConfig == nil || len(*configs.ALLConfig) != 0 || configs.EnablesConfig != nil {
				t.Fatalf("unexpected first-launch configuration: %+v", configs)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var stored map[string]json.RawMessage
			if err := json.Unmarshal(data, &stored); err != nil || string(stored["config"]) != "[]" || string(stored["EnablesConfig"]) != "null" {
				t.Fatalf("unexpected initial configuration file: %s, %v", data, err)
			}
			if _, err := os.Stat(file + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("temporary configuration was not moved into place: %v", err)
			}
		})
	}
}

func TestFirstLaunchCanAddModelConfig(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data", "config", "MainConfig.json")
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	configs, err := backend.Platform.AddModelConfig("Test Provider", "https://example.test/v1/chat/completions", "chat-completions", "test-key", []apiImpl.Model{{ID: "model-1", MaxContext: 128000}})
	if err != nil {
		t.Fatal(err)
	}
	if configs.EnablesConfig == nil || configs.EnablesConfig.EnabledModel != "model-1" {
		t.Fatalf("first provider was not enabled: %+v", configs)
	}
	backend.Platform.Configs = nil
	configs, err = backend.Platform.LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(*configs.ALLConfig) != 1 || configs.EnablesConfig == nil || configs.EnablesConfig.Name != "Test Provider" || configs.EnablesConfig.Model[0].MaxContext != 128000 {
		t.Fatalf("first provider was not persisted: %+v", configs)
	}
}

func TestLoadConfigsUsesReceiver(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[]}`))
	instance := &backend.Core{
		Paths: &platform.Paths{ConfigFile: writeConfig(t, `{"config":[{"ID":"provider","Name":"Local Provider","Model":[{"id":"model"}]}]}`)},
		FS:    &desktopBackend.DesktopFileSystem{},
	}
	configs, err := instance.LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if configs == nil || len(*configs.ALLConfig) != 1 || (*configs.ALLConfig)[0].Name != "Local Provider" || backend.Platform.Configs != nil {
		t.Fatal("loading configuration used the global Core instead of the receiver")
	}
	if _, err := instance.RenameConfig(0, "Renamed Provider"); err != nil {
		t.Fatal(err)
	}
	configs, err = instance.LoadConfigs()
	if err != nil || (*configs.ALLConfig)[0].Name != "Renamed Provider" {
		t.Fatalf("saving configuration used the wrong Core: %+v, %v", configs, err)
	}
}

type unreadableConfigFS struct{ platform.FilePlatform }

func (unreadableConfigFS) ReadFile(string) ([]byte, error) { return nil, fs.ErrPermission }

func TestLoadConfigsReportsReadFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "MainConfig.json")
	useConfigFile(t, file)
	backend.Platform.FS = unreadableConfigFS{backend.Platform.FS}
	if _, err := backend.Platform.LoadConfigs(); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("expected read error, got %v", err)
	}
	if backend.Platform.Configs != nil {
		t.Fatal("read failure was treated as a missing configuration")
	}
}

func TestLoadConfigsReportsCreationFailure(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "data")
	if err := os.WriteFile(blocked, []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	useConfigFile(t, filepath.Join(blocked, "config", "MainConfig.json"))
	if _, err := backend.Platform.LoadConfigs(); err == nil {
		t.Fatal("configuration creation failure was ignored")
	}
	if backend.Platform.Configs != nil {
		t.Fatal("failed creation left a configuration marked as loaded")
	}
}

func TestLoadConfigsRequiresStorage(t *testing.T) {
	for _, instance := range []*backend.Core{
		nil,
		{},
		{Paths: &platform.Paths{}},
		{Paths: &platform.Paths{ConfigFile: filepath.Join(t.TempDir(), "MainConfig.json")}},
	} {
		if _, err := instance.LoadConfigs(); err == nil {
			t.Fatal("uninitialized configuration storage was accepted")
		}
	}
}

func TestSelectConfigPersistsChoice(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	selected, err := backend.Platform.SelectConfig(1)
	if err != nil {
		t.Fatal(err)
	}
	if selected.EnablesConfig == nil || selected.EnablesConfig.ServerAddress != "https://second" {
		t.Fatalf("unexpected selection: %+v", selected)
	}

	// 重新加载后选择应保留。
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	enabled := backend.Platform.Configs.EnablesConfig
	if enabled == nil || enabled.ServerAddress != "https://second" {
		t.Fatalf("selection not persisted: %+v", enabled)
	}
}

func TestSelectConfigRejectsBadIndex(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	for _, index := range []int{-1, 1, 99} {
		if _, err := backend.Platform.SelectConfig(index); err == nil {
			t.Fatalf("index %d accepted", index)
		}
	}
	if backend.Platform.Configs.EnablesConfig != nil {
		t.Fatal("failed selection should not enable anything")
	}
}

func TestSelectModelPersistsChoice(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectModel("m3"); err != nil {
		t.Fatal(err)
	}

	// 重新加载后提供商和模型都应保留，且启用项指回数组元素。
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	cfg := backend.Platform.Configs
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.ServerAddress != "https://second" || cfg.EnablesConfig.EnabledModel != "m3" {
		t.Fatalf("unexpected state: %+v", cfg)
	}
	if cfg.EnablesConfig != &(*cfg.ALLConfig)[1] {
		t.Fatal("enabled config not resolved to list entry")
	}
	if (*cfg.ALLConfig)[1].EnabledModel != "m3" {
		t.Fatalf("model not persisted into config entry: %+v", (*cfg.ALLConfig)[1])
	}
}

func TestSelectModelRequiresProvider(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.Platform.SelectModel("m1"); err == nil {
		t.Fatal("model accepted without enabled config")
	}
}

func TestSelectModelRejectsUnknownModel(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.Platform.SelectModel("nope"); err == nil {
		t.Fatal("unknown model accepted")
	}
	if backend.Platform.Configs.EnablesConfig.EnabledModel != "" {
		t.Fatal("failed model selection should not stick")
	}
}

func TestSelectConfigRemembersModelPerProvider(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	// 各选各的模型。
	if _, err := backend.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectModel("m1"); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectModel("m2"); err != nil {
		t.Fatal(err)
	}

	// 来回切换：启用模型随配置走，互不覆盖。
	if _, err := backend.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}
	if got := backend.Platform.Configs.EnablesConfig.EnabledModel; got != "m1" {
		t.Fatalf("first provider forgot its model: %q", got)
	}
	if _, err := backend.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if got := backend.Platform.Configs.EnablesConfig.EnabledModel; got != "m2" {
		t.Fatalf("second provider forgot its model: %q", got)
	}

	// 重新加载后依然成立。
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	cfg := backend.Platform.Configs
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.EnabledModel != "m2" {
		t.Fatalf("unexpected state after reload: %+v", cfg)
	}
}

func TestLoadConfigDefaultsToFirstModel(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1","m2"]}
	],"EnablesConfig":{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1","m2"]}}`)
	useConfigFile(t, file)

	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	enabled := backend.Platform.Configs.EnablesConfig
	if enabled == nil || enabled.EnabledModel != "m1" {
		t.Fatalf("default model not applied: %+v", enabled)
	}
}

func TestLoadConfigMigratesLegacyModelNames(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1","m2"]}]}`)
	useConfigFile(t, file)

	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	models := (*backend.Platform.Configs.ALLConfig)[0].Model
	if len(models) != 2 || models[0].ID != "m1" || models[0].Name != "m1" {
		t.Fatalf("legacy models not converted: %+v", models)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"Model": ["m1"`) || !strings.Contains(string(data), `"id": "m1"`) {
		t.Fatalf("legacy model config was not migrated on disk: %s", data)
	}
}

func TestAddModelConfigPersistsModelMetadata(t *testing.T) {
	file := writeConfig(t, `{"config":[]}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	configs, err := backend.Platform.AddModelConfig("测试服务", "https://example.com/v1", "chat-completions", "sk-test", []apiImpl.Model{{
		ID:              "m1",
		Name:            "Model One",
		MaxContext:      32768,
		MaxOutputTokens: 4096,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if configs.EnablesConfig == nil || configs.EnablesConfig.EnabledModel != "m1" {
		t.Fatalf("new model config was not enabled: %+v", configs.EnablesConfig)
	}
	if got := configs.EnablesConfig.Model[0].MaxContext; got != 32768 {
		t.Fatalf("model metadata not retained: %d", got)
	}
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	if got := backend.Platform.Configs.EnablesConfig.Model[0].ID; got != "m1" {
		t.Fatalf("model config not persisted: %q", got)
	}
}

func TestRefreshModelConfigPersistsUpdatedModels(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://old.example/v1/chat/completions","APIKey":"sk-test","Model":[{"id":"keep","name":"Keep"},{"id":"remove","name":"Remove"}],"EnabledModel":"keep"}],"EnablesConfig":{"ServerAddress":"https://old.example/v1/chat/completions","APIKey":"sk-test","Model":[{"id":"keep","name":"Keep"},{"id":"remove","name":"Remove"}],"EnabledModel":"keep"}}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	configs, err := backend.Platform.RefreshModelConfig(0, "https://new.example/chat/completions", []apiImpl.Model{{ID: "keep", Name: "Keep updated"}, {ID: "new", Name: "New"}})
	if err != nil {
		t.Fatal(err)
	}
	entry := (*configs.ALLConfig)[0]
	if entry.ServerAddress != "https://new.example/chat/completions" || entry.EnabledModel != "keep" || len(entry.Model) != 2 {
		t.Fatalf("unexpected refreshed config: %+v", entry)
	}
	if configs.EnablesConfig != &(*configs.ALLConfig)[0] {
		t.Fatal("enabled config no longer points to refreshed entry")
	}
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	if got := backend.Platform.Configs.EnablesConfig; got == nil || got.ServerAddress != "https://new.example/chat/completions" || got.EnabledModel != "keep" {
		t.Fatalf("refreshed config not persisted: %+v", got)
	}
}

func TestRefreshModelConfigRejectsEmptyModelsWithoutChanges(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://old.example","APIKey":"sk-test","Model":[{"id":"keep"}],"EnabledModel":"keep"}]}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.Platform.RefreshModelConfig(0, "https://new.example", nil); err == nil {
		t.Fatal("empty model list accepted")
	}
	entry := (*backend.Platform.Configs.ALLConfig)[0]
	if entry.ServerAddress != "https://old.example" || len(entry.Model) != 1 || entry.EnabledModel != "keep" {
		t.Fatalf("failed refresh changed existing config: %+v", entry)
	}
}

func TestLoadConfigToleratesNoEnabledEntry(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}
	]}`))

	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	if backend.Platform.Configs.EnablesConfig != nil {
		t.Fatal("unexpected enabled entry")
	}
}

func TestLoadConfigDropsStaleEnabledEntry(t *testing.T) {
	// 顶层的 EnabledModel 是旧版遗留字段，应被忽略；幽灵启用项应被清空。
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}
	],"EnablesConfig":{"ServerAddress":"https://ghost","APIKey":"sk-x","Model":["m9"]},"EnabledModel":"m9"}`)
	useConfigFile(t, file)

	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	cfg := backend.Platform.Configs
	if cfg.EnablesConfig != nil {
		t.Fatalf("stale enabled entry kept: %+v", cfg)
	}
}

func TestRenameConfigPersistsChoice(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}

	state, err := backend.Platform.RenameConfig(0, "  主力配置  ")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*state.ALLConfig)[0].Name; got != "主力配置" {
		t.Fatalf("name not trimmed: %q", got)
	}

	// 重新加载后名称保留，且启用项同步带上了新名称。
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	cfg := backend.Platform.Configs
	if (*cfg.ALLConfig)[0].Name != "主力配置" {
		t.Fatalf("rename not persisted: %+v", (*cfg.ALLConfig)[0])
	}
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.Name != "主力配置" {
		t.Fatalf("enabled entry missed rename: %+v", cfg.EnablesConfig)
	}
}

func TestRenameConfigRejectsBadIndex(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if _, err := backend.Platform.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	for _, index := range []int{-1, 1} {
		if _, err := backend.Platform.RenameConfig(index, "x"); err == nil {
			t.Fatalf("index %d accepted", index)
		}
	}
	if (*backend.Platform.Configs.ALLConfig)[0].Name != "" {
		t.Fatal("failed rename should not change anything")
	}
}
