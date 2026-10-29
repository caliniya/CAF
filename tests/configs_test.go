package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	desktopBackend "github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/platform"
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
	core.Platform = &core.Core{
		Paths: &platform.Paths{ConfigFile: file},
		FS:    &desktopBackend.DesktopFileSystem{},
	}
}

func TestLoadConfigReadsFile(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://example.com","APIKey":"sk-test","Model":["m1","m2"]}]}`)
	useConfigFile(t, file)

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	got := core.Platform.Configs
	if got == nil || got.ALLConfig == nil || len(*got.ALLConfig) != 1 {
		t.Fatalf("unexpected configs: %+v", got)
	}
	entry := (*got.ALLConfig)[0]
	if entry.ServerAddress != "https://example.com" || entry.APIKey != "sk-test" || len(entry.Model) != 2 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestLoadConfigReportsMalformedJSON(t *testing.T) {
	useConfigFile(t, writeConfig(t, "{"))

	if err := core.LoadConfig(); err == nil {
		t.Fatal("malformed json accepted")
	}
}

func TestLoadConfigReportsMissingFile(t *testing.T) {
	useConfigFile(t, filepath.Join(t.TempDir(), "MainConfig.json"))

	if err := core.LoadConfig(); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestSelectConfigPersistsChoice(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	selected, err := core.Platform.SelectConfig(1)
	if err != nil {
		t.Fatal(err)
	}
	if selected.EnablesConfig == nil || selected.EnablesConfig.ServerAddress != "https://second" {
		t.Fatalf("unexpected selection: %+v", selected)
	}

	// 重新加载后选择应保留。
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	enabled := core.Platform.Configs.EnablesConfig
	if enabled == nil || enabled.ServerAddress != "https://second" {
		t.Fatalf("selection not persisted: %+v", enabled)
	}
}

func TestSelectConfigRejectsBadIndex(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	for _, index := range []int{-1, 1, 99} {
		if _, err := core.Platform.SelectConfig(index); err == nil {
			t.Fatalf("index %d accepted", index)
		}
	}
	if core.Platform.Configs.EnablesConfig != nil {
		t.Fatal("failed selection should not enable anything")
	}
}

func TestSelectModelPersistsChoice(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectModel("m3"); err != nil {
		t.Fatal(err)
	}

	// 重新加载后提供商和模型都应保留，且启用项指回数组元素。
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := core.Platform.Configs
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
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	if _, err := core.Platform.SelectModel("m1"); err == nil {
		t.Fatal("model accepted without enabled config")
	}
}

func TestSelectModelRejectsUnknownModel(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}

	if _, err := core.Platform.SelectModel("nope"); err == nil {
		t.Fatal("unknown model accepted")
	}
	if core.Platform.Configs.EnablesConfig.EnabledModel != "" {
		t.Fatal("failed model selection should not stick")
	}
}

func TestSelectConfigRemembersModelPerProvider(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://first","APIKey":"sk-1","Model":["m1"]},
		{"ServerAddress":"https://second","APIKey":"sk-2","Model":["m2","m3"]}
	]}`)
	useConfigFile(t, file)
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	// 各选各的模型。
	if _, err := core.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectModel("m1"); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectModel("m2"); err != nil {
		t.Fatal(err)
	}

	// 来回切换：启用模型随配置走，互不覆盖。
	if _, err := core.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}
	if got := core.Platform.Configs.EnablesConfig.EnabledModel; got != "m1" {
		t.Fatalf("first provider forgot its model: %q", got)
	}
	if _, err := core.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if got := core.Platform.Configs.EnablesConfig.EnabledModel; got != "m2" {
		t.Fatalf("second provider forgot its model: %q", got)
	}

	// 重新加载后依然成立。
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := core.Platform.Configs
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.EnabledModel != "m2" {
		t.Fatalf("unexpected state after reload: %+v", cfg)
	}
}

func TestLoadConfigDefaultsToFirstModel(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1","m2"]}
	],"EnablesConfig":{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1","m2"]}}`)
	useConfigFile(t, file)

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	enabled := core.Platform.Configs.EnablesConfig
	if enabled == nil || enabled.EnabledModel != "m1" {
		t.Fatalf("default model not applied: %+v", enabled)
	}
}

func TestLoadConfigMigratesLegacyModelNames(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1","m2"]}]}`)
	useConfigFile(t, file)

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	models := (*core.Platform.Configs.ALLConfig)[0].Model
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
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	configs, err := core.Platform.AddModelConfig("测试服务", "https://example.com/v1", "chat-completions", "sk-test", []apiImpl.Model{{
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
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got := core.Platform.Configs.EnablesConfig.Model[0].ID; got != "m1" {
		t.Fatalf("model config not persisted: %q", got)
	}
}

func TestRefreshModelConfigPersistsUpdatedModels(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://old.example/v1/chat/completions","APIKey":"sk-test","Model":[{"id":"keep","name":"Keep"},{"id":"remove","name":"Remove"}],"EnabledModel":"keep"}],"EnablesConfig":{"ServerAddress":"https://old.example/v1/chat/completions","APIKey":"sk-test","Model":[{"id":"keep","name":"Keep"},{"id":"remove","name":"Remove"}],"EnabledModel":"keep"}}`)
	useConfigFile(t, file)
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	configs, err := core.Platform.RefreshModelConfig(0, "https://new.example/chat/completions", []apiImpl.Model{{ID: "keep", Name: "Keep updated"}, {ID: "new", Name: "New"}})
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
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got := core.Platform.Configs.EnablesConfig; got == nil || got.ServerAddress != "https://new.example/chat/completions" || got.EnabledModel != "keep" {
		t.Fatalf("refreshed config not persisted: %+v", got)
	}
}

func TestRefreshModelConfigRejectsEmptyModelsWithoutChanges(t *testing.T) {
	file := writeConfig(t, `{"config":[{"ServerAddress":"https://old.example","APIKey":"sk-test","Model":[{"id":"keep"}],"EnabledModel":"keep"}]}`)
	useConfigFile(t, file)
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	if _, err := core.Platform.RefreshModelConfig(0, "https://new.example", nil); err == nil {
		t.Fatal("empty model list accepted")
	}
	entry := (*core.Platform.Configs.ALLConfig)[0]
	if entry.ServerAddress != "https://old.example" || len(entry.Model) != 1 || entry.EnabledModel != "keep" {
		t.Fatalf("failed refresh changed existing config: %+v", entry)
	}
}

func TestLoadConfigToleratesNoEnabledEntry(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}
	]}`))

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if core.Platform.Configs.EnablesConfig != nil {
		t.Fatal("unexpected enabled entry")
	}
}

func TestLoadConfigDropsStaleEnabledEntry(t *testing.T) {
	// 顶层的 EnabledModel 是旧版遗留字段，应被忽略；幽灵启用项应被清空。
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}
	],"EnablesConfig":{"ServerAddress":"https://ghost","APIKey":"sk-x","Model":["m9"]},"EnabledModel":"m9"}`)
	useConfigFile(t, file)

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := core.Platform.Configs
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
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Platform.SelectConfig(0); err != nil {
		t.Fatal(err)
	}

	state, err := core.Platform.RenameConfig(0, "  主力配置  ")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*state.ALLConfig)[0].Name; got != "主力配置" {
		t.Fatalf("name not trimmed: %q", got)
	}

	// 重新加载后名称保留，且启用项同步带上了新名称。
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := core.Platform.Configs
	if (*cfg.ALLConfig)[0].Name != "主力配置" {
		t.Fatalf("rename not persisted: %+v", (*cfg.ALLConfig)[0])
	}
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.Name != "主力配置" {
		t.Fatalf("enabled entry missed rename: %+v", cfg.EnablesConfig)
	}
}

func TestRenameConfigRejectsBadIndex(t *testing.T) {
	useConfigFile(t, writeConfig(t, `{"config":[{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}]}`))
	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	for _, index := range []int{-1, 1} {
		if _, err := core.Platform.RenameConfig(index, "x"); err == nil {
			t.Fatalf("index %d accepted", index)
		}
	}
	if (*core.Platform.Configs.ALLConfig)[0].Name != "" {
		t.Fatal("failed rename should not change anything")
	}
}
