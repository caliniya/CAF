package core

import (
	"encoding/json"
	"os"

	netclient "github.com/caliniya/caf/NetClient"
	"github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	QQPlatform platform.QQPlatform
	Paths      *platform.Paths
	Configs    *Configs
	APIClients *[]netclient.APIClient
}

type Configs struct {
	ALLConfig     *[]Config `json:"config"`
	EnablesConfig *Config
}

type Config struct {
	Name          string   `json:"Name"`
	ServerAddress string   `json:"ServerAddress"`
	APIKey        string   `json:"APIKey"`
	Model         []string `json:"Model"`
	EnabledModel  string   `json:"EnabledModel"`
}

func Init() {
	Platform = &Core{}
}

func LoadConfig() error {
	data, err := os.ReadFile(Platform.Paths.ConfigFile)
	if err != nil {
		return err
	}
	Platform.Configs = &Configs{}
	if err := json.Unmarshal(data, Platform.Configs); err != nil {
		return err
	}
	resolveEnabled()

	// 启用项没记住模型时默认选第一个，开箱即用；仅在内存中生效，随下次保存落盘。
	if cfg := Platform.Configs.EnablesConfig; cfg != nil && cfg.EnabledModel == "" && len(cfg.Model) > 0 {
		cfg.EnabledModel = cfg.Model[0]
	}
	return nil
}

// resolveEnabled 把文件里的 EnablesConfig（一份拷贝）指回配置数组里的对应项，
// 后续选模型、写回时才不会和配置列表脱节；找不到对应项则视为过期数据，一并清空。
func resolveEnabled() {
	cfg := Platform.Configs
	if cfg.EnablesConfig == nil {
		return
	}
	if cfg.ALLConfig != nil {
		for i := range *cfg.ALLConfig {
			if sameConfig(&(*cfg.ALLConfig)[i], cfg.EnablesConfig) {
				cfg.EnablesConfig = &(*cfg.ALLConfig)[i]
				return
			}
		}
	}
	cfg.EnablesConfig = nil
}

// sameConfig 名称不参与比较：改名不影响启用项的归位。
func sameConfig(a, b *Config) bool {
	if a.ServerAddress != b.ServerAddress || a.APIKey != b.APIKey || len(a.Model) != len(b.Model) {
		return false
	}
	for i := range a.Model {
		if a.Model[i] != b.Model[i] {
			return false
		}
	}
	return true
}

// SaveConfig 把当前配置（含启用项）写回配置文件，先写临时文件再替换，避免写一半损坏原文件。
func SaveConfig() error {
	data, err := json.MarshalIndent(Platform.Configs, "", "    ")
	if err != nil {
		return err
	}
	tmp := Platform.Paths.ConfigFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Platform.Paths.ConfigFile)
}

func (c *Core) GetPaths() *platform.Paths {
	return c.Paths // Paths 有 json tag,可以直接序列化过去
}
