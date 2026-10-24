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
	json.Unmarshal(data, Platform.Configs)
	return nil
}

func (c *Core) GetPaths() *platform.Paths {
	return c.Paths // Paths 有 json tag,可以直接序列化过去
}
