package core

import (
	"encoding/json"
	"os"

	"github.com/caliniya/caf/platform"
)

var Platform *Core

type Core struct {
	QQPlatform platform.QQPlatform
	Paths      *platform.Paths
	Configs    *Configs
}

type Configs struct {
	ALLConfig     *[]Config `json:"config"`
	EnablesConfig *Config
}

type Config struct {
	ServerAddress string   `json:"ServerAddress"`
	APIKey        string   `json:"APIKey"`
	Model         []string `json:"Model"`
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
	return json.Unmarshal(data, Platform.Configs)
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
