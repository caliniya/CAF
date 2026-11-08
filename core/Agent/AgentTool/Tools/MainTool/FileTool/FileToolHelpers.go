package FileToolInMainTool

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	core "github.com/caliniya/caf/core"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	platform "github.com/caliniya/caf/platform"
)

func pathParameter() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": `按顺序排列的路径片段数组。相对路径以当前工作区为基准；Windows 绝对路径示例：["C:\", "Users", "Alice", "file.txt"]。当前工作区目录可传 ["."]。`,
		"minItems":    1,
		"items": map[string]any{
			"type":      "string",
			"minLength": 1,
		},
	}
}

func resolveFilePath(ctx *agenttool.ToolContext, arg map[string]any) (string, error) {
	value, present := arg["path"]
	if !present || value == nil {
		return "", errors.New("缺少参数 path")
	}

	var parts []string
	switch value := value.(type) {
	case []string:
		parts = value
	case []any:
		parts = make([]string, len(value))
		for i, part := range value {
			text, ok := part.(string)
			if !ok {
				return "", fmt.Errorf("参数 path[%d] 必须是非空字符串", i)
			}
			parts[i] = text
		}
	default:
		return "", errors.New("参数 path 必须是非空字符串数组")
	}
	if len(parts) == 0 {
		return "", errors.New("参数 path 必须是非空字符串数组")
	}
	for i, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("参数 path[%d] 必须是非空字符串", i)
		}
	}

	path := filepath.Join(parts...)
	if ctx != nil {
		return ctx.ResolvePath(path)
	}
	return path, nil
}

func fileSystem() (platform.FilePlatform, error) {
	if core.Platform == nil || core.Platform.FS == nil {
		return nil, errors.New("文件系统尚未初始化")
	}
	return core.Platform.FS, nil
}
