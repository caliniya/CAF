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
	t.ToolMeta.Description = "读取指定文件并返回其文本内容。path 是按顺序排列的路径片段；相对路径以当前工作区为基准。可用 start_line/end_line 读取指定行范围，行号从 1 开始且包含结束行；均不传时读取整个文件。保留原始换行符，末尾换行不额外计为空行；结束行超出文件时读取到末尾，起始行超出文件时返回空字符串。"
	t.ToolMeta.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       pathParameter(),
			"start_line": lineParameter("可选的起始行号，从 1 开始，省略时默认从第 1 行读取。"),
			"end_line":   lineParameter("可选的结束行号，包含该行；省略时读取到文件末尾，不能小于 start_line。"),
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
	lineRange, err := parseFileLineRange(arg)
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
	if lineRange.specified {
		start, end, _ := lineRange.offsets(data)
		return string(data[start:end]), nil
	}
	return string(data), nil
}

func (t ReadFile) ToType() *APIImpl.Tool {
	return t.ToolMeta.ToType()
}
