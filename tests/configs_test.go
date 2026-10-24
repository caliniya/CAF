package tests

import (
	"os"
	"path/filepath"
	"testing"

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
	core.Platform = &core.Core{Paths: &platform.Paths{ConfigFile: file}}
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
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.ServerAddress != "https://second" || cfg.EnabledModel != "m3" {
		t.Fatalf("unexpected state: %+v", cfg)
	}
	if cfg.EnablesConfig != &(*cfg.ALLConfig)[1] {
		t.Fatal("enabled config not resolved to list entry")
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
	if core.Platform.Configs.EnabledModel != "" {
		t.Fatal("failed model selection should not stick")
	}
}

func TestSelectConfigClearsStaleModel(t *testing.T) {
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
	if _, err := core.Platform.SelectModel("m1"); err != nil {
		t.Fatal(err)
	}

	// m1 不在第二个配置的模型列表里，切换提供商后应清空模型选择。
	if _, err := core.Platform.SelectConfig(1); err != nil {
		t.Fatal(err)
	}
	if core.Platform.Configs.EnabledModel != "" {
		t.Fatalf("stale model kept: %q", core.Platform.Configs.EnabledModel)
	}

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := core.Platform.Configs
	if cfg.EnablesConfig == nil || cfg.EnablesConfig.ServerAddress != "https://second" || cfg.EnabledModel != "" {
		t.Fatalf("unexpected state after reload: %+v", cfg)
	}
}

func TestLoadConfigDropsStaleEnabledEntry(t *testing.T) {
	file := writeConfig(t, `{"config":[
		{"ServerAddress":"https://only","APIKey":"sk-1","Model":["m1"]}
	],"EnablesConfig":{"ServerAddress":"https://ghost","APIKey":"sk-x","Model":["m9"]},"EnabledModel":"m9"}`)
	useConfigFile(t, file)

	if err := core.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := core.Platform.Configs
	if cfg.EnablesConfig != nil || cfg.EnabledModel != "" {
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
