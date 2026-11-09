package FileToolInMainTool

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	backend "github.com/caliniya/caf/core/Backend"
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

func lineParameter(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
		"minimum":     1,
	}
}

type fileLineRange struct {
	start, end int // end 为 0 表示直到文件末尾。
	specified  bool
}

func parseFileLineRange(arg map[string]any) (fileLineRange, error) {
	result := fileLineRange{start: 1}
	for _, parameter := range []struct {
		name   string
		target *int
	}{{"start_line", &result.start}, {"end_line", &result.end}} {
		value, present := arg[parameter.name]
		if !present {
			continue
		}
		result.specified = true
		invalid := fmt.Errorf("参数 %s 必须是从 1 开始的整数行号", parameter.name)
		switch value := value.(type) {
		case int:
			if value < 1 {
				return fileLineRange{}, invalid
			}
			*parameter.target = value
		case float64:
			// JSON 数字通常解码为 float64；转换前排除小数和 int 溢出。
			if math.IsNaN(value) || value < 1 || value >= float64(int(^uint(0)>>1)) || math.Trunc(value) != value {
				return fileLineRange{}, invalid
			}
			*parameter.target = int(value)
		default:
			return fileLineRange{}, invalid
		}
	}
	if result.end != 0 && result.end < result.start {
		return fileLineRange{}, errors.New("参数 end_line 不能小于 start_line")
	}
	return result, nil
}

// offsets 返回包含行尾换行符的字节范围；末尾换行不额外计为空行。
// 直接按字节定位，保留 CRLF、空行和超过 Scanner 默认限制的长行。
func (r fileLineRange) offsets(data []byte) (start, end, lineCount int) {
	start, end = len(data), len(data)
	for offset := 0; offset < len(data); {
		lineCount++
		if lineCount == r.start {
			start = offset
		}
		next := len(data)
		if index := bytes.IndexByte(data[offset:], '\n'); index >= 0 {
			next = offset + index + 1
		}
		if lineCount == r.end {
			end = next
		}
		offset = next
	}
	return start, end, lineCount
}

func (r fileLineRange) replace(data []byte, content string) ([]byte, error) {
	start, end, lineCount := r.offsets(data)
	if r.start > lineCount || r.end > lineCount {
		return nil, fmt.Errorf("写入行范围超出文件的 %d 行", lineCount)
	}
	if content != "" {
		// 新内容沿用选中范围的换行风格，并保留替换范围与后续行的边界。
		newline := "\n"
		index := bytes.IndexByte(data[start:end], '\n')
		if (index > 0 && data[start+index-1] == '\r') || (index < 0 && start >= 2 && data[start-2] == '\r') {
			newline = "\r\n"
		}
		content = strings.ReplaceAll(content, "\r\n", "\n")
		if newline == "\r\n" {
			content = strings.ReplaceAll(content, "\n", newline)
		}
		if data[end-1] == '\n' && !strings.HasSuffix(content, "\n") {
			content += newline
		}
	}
	result := make([]byte, 0, start+len(content)+len(data)-end)
	result = append(result, data[:start]...)
	result = append(result, content...)
	result = append(result, data[end:]...)
	return result, nil
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
	if backend.Platform == nil || backend.Platform.FS == nil {
		return nil, errors.New("文件系统尚未初始化")
	}
	return backend.Platform.FS, nil
}
