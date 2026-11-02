package agenttool

var Tools []Tool

func Add(t Tool) {
	Tools = append(Tools, t)
}
