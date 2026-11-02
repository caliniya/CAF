package agenttool

var Tools []ToolMeta

func Add(t ToolMeta) {
	Tools = append(Tools, t)
}
