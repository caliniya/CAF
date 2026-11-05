package FileToolInMainTool

import agenttool "github.com/caliniya/caf/core/Agent/AgentTool"

type FileTool struct {
	agenttool.ToolKit
}

// Description implements [agenttool.ToolsSet].
func (f *FileTool) Description() string {
	panic("unimplemented")
}

// DisEnableSet implements [agenttool.ToolsSet].
func (f *FileTool) DisEnableSet(string) error {
	panic("unimplemented")
}

// EnableSet implements [agenttool.ToolsSet].
func (f *FileTool) EnableSet(string) error {
	panic("unimplemented")
}

// Get implements [agenttool.ToolsSet].
func (f *FileTool) Get(string) agenttool.ToolKit {
	panic("unimplemented")
}

// List implements [agenttool.ToolsSet].
func (f *FileTool) List() map[string]agenttool.ToolKit {
	panic("unimplemented")
}

// Name implements [agenttool.ToolsSet].
func (f *FileTool) Name() string {
	panic("unimplemented")
}
