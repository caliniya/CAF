package FileToolInMainTool

import "github.com/caliniya/caf/core/Agent/AgentTool"

type FileTool struct {
	*AgentTool.ToolKitMeta
}

// List implements [AgentTool.ToolKit].
func (f *FileTool) List() []AgentTool.Tool {
	temp := []AgentTool.Tool{}
	for _, v := range f.Tools {
		temp = append(temp, v)
	}
	return temp
}

// ListName implements [AgentTool.ToolKit].
func (f *FileTool) ListName() map[string]AgentTool.Tool {
	return f.Tools
}

// Meta implements [AgentTool.ToolKit].
func (f *FileTool) Meta() AgentTool.ToolKitMeta {
	return *f.ToolKitMeta
}

func InitFileTool() *FileTool {
	ReadFileTool := ReadFile{}
	ReadFileTool.Load()

	return &FileTool{ToolKitMeta: &AgentTool.ToolKitMeta{
		Name:        "文件操作工具",
		Description: "标准工具集的一部分,提供文件操作能力",
		Enabled:     true,
		Tools: map[string]AgentTool.Tool{
			ReadFileTool.Name: &ReadFileTool,
		},
	}}
}
