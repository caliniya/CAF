package FileToolInMainTool

import (
	"github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type ReadFile struct {
	agenttool.ToolMeta
}

func (t *ReadFile) Load() {
	t.ToolMeta.ID = "ReadFile"
	t.ToolMeta.Name = "读取文件"
	t.ToolMeta.TimeoutParam = "timeout_seconds"
	t.ToolMeta.Description = "读取指定文件并返回其文本内容。path 是按顺序排列的路径片段；相对路径以当前工作区为基准。"
	t.ToolMeta.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": pathParameter(),
		},
		"required": []string{"path"},
	}
	t.ToolMeta.Enabled = true
	t.ToolMeta.Permission = agenttool.PermissionReadOnly
}

func (t *ReadFile) Meta() *agenttool.ToolMeta {
	return &t.ToolMeta
}

func (t *ReadFile) Fun(ctx *agenttool.ToolContext, arg map[string]any) (string, error) {
	path, err := resolveFilePath(ctx, arg)
	if err != nil {
		return "", err
	}
	fsys, err := fileSystem()
	if err != nil {
		return "", err
	}
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t ReadFile) ToType() *APIImpl.Tool {
	return t.ToolMeta.ToType()
}
