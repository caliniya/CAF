package agenttool

var Tools []Tool

var ToolSets map[string]ToolSet

func Add(t ToolSet) {
	if ToolSets == nil {
		ToolSets = make(map[string]ToolSet)
	}
	ToolSets[t.Name] = t
}

func Sync() {

}
