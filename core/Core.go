package core

import "github.com/caliniya/caf/platform"

var Platform *Core

type Core struct {
	QQ    *platform.QQ
	Paths *platform.Paths
}

func Init() {
	Platform = &Core{}
}
