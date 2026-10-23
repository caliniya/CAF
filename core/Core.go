package core

import "github.com/caliniya/caf/platform"

var Platform *Core

type Core struct {
	QQPlatform platform.QQPlatform
	Paths      *platform.Paths
	Configs    *Configs
}

type Configs struct {
	ALLConfig     *[]Config
	EnablesConfig *Config
}

type Config struct {
	ServerAddress string `json:"ServerAddress"`
	APIKey        string `json:"APIKey"`
	Modle         string
}

func Init() {
	Platform = &Core{}
}
