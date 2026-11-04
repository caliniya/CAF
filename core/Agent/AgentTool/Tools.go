package agenttool

import apiImpl "github.com/caliniya/caf/NetClient/APIImpl"

var Tools []Tool

var ToolSets map[string]ToolSet

var ToolList map[string]apiImpl.Tool

func Init() {
	Tools = []Tool{}
	ToolSets = map[string]ToolSet{}
	ToolList = map[string]apiImpl.Tool{}
}

func Add(t ToolSet) {
	if ToolSets == nil {
		ToolSets = make(map[string]ToolSet)
	}
	ToolSets[t.Name] = t
}

func Sync() {
	for t := range Tools {
		meta := Tools[t].Meta()
		ToolList[meta.Name] = apiImpl.Tool{
			Name:        meta.Name,
			Description: meta.Description,
			Parameters:  meta.Parameters,
		}
	}
}
