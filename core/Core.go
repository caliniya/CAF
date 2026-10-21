package core

import "github.com/caliniya/caf/platform"

var Platform *Core

type Core struct {
	QQPlatform platform.QQPlatform
	Paths      *platform.Paths
}

func Init() {
	Platform = &Core{}
}
