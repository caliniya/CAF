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
	if selected.ServerAddress != "https://second" || len(selected.Model) != 2 {
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
