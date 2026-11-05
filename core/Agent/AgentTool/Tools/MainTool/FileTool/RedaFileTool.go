package FileToolInMainTool

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/caliniya/caf/core"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type ReadFile struct {
	agenttool.ToolMeta
}

func (t *ReadFile) Load() {
	t.ToolMeta.Name = "ReadFile"
	t.ToolMeta.Description = `读取指定文件并返回其文本内容。path 是按顺序排列的路径片段；Windows 绝对路径的盘符片段可写为 "C:\"`
	t.ToolMeta.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "array",
				"description": `路径片段数组，例如 Windows 绝对路径可传 ["C:\", "Users", "Alice", "file.txt"]。`,
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFile) Meta() *agenttool.ToolMeta {
	return &t.ToolMeta
}

func (t *ReadFile) Fun(arg map[string]any) (string, error) {
	pathValue, ok := arg["path"]
	if !ok || pathValue == nil {
		return "", errors.New("缺少参数 path")
	}

	pathParts, ok := pathValue.([]any)
	if !ok || len(pathParts) == 0 {
		return "", errors.New("参数 path 必须是非空字符串数组")
	}

	parts := make([]string, len(pathParts))
	for i, value := range pathParts {
		part, ok := value.(string)
		if !ok || strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("参数 path[%d] 必须是非空字符串", i)
		}
		parts[i] = part
	}
	path := filepath.Join(parts...)

	data, err := core.Platform.FS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
