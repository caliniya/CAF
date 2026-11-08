package FileToolInMainTool

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type SearchFiles struct {
	agenttool.ToolMeta
}

func (t *SearchFiles) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID:           "SearchFiles",
		Name:         "搜索文件",
		Description:  "在指定文件夹中按文件名搜索文件。pattern 支持 *、?、[...] 通配符；没有通配符时按文件名包含匹配。默认递归且不区分大小写，不跟随符号链接目录。返回 JSON，包含匹配路径和是否因数量限制被截断。",
		TimeoutParam: "timeout_seconds",
		Permission:   agenttool.PermissionReadOnly,
		Enabled:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": pathParameter(),
				"pattern": map[string]any{
					"type":        "string",
					"description": "文件名关键词或通配符，例如 report、*.go、test_?.txt；仅匹配文件名，不搜索文件内容。",
					"minLength":   1,
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "是否搜索子文件夹。",
					"default":     true,
				},
				"case_sensitive": map[string]any{
					"type":        "boolean",
					"description": "文件名匹配是否区分大小写。",
					"default":     false,
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "最多返回的文件数量。",
					"minimum":     1,
					"maximum":     10000,
					"default":     200,
				},
			},
			"required": []string{"path", "pattern"},
		},
	}
}

func (t *SearchFiles) Meta() *agenttool.ToolMeta {
	return &t.ToolMeta
}

func (t *SearchFiles) Fun(ctx *agenttool.ToolContext, arg map[string]any) (string, error) {
	path, err := resolveFilePath(ctx, arg)
	if err != nil {
		return "", err
	}
	pattern, ok := arg["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		return "", errors.New("参数 pattern 必须是非空字符串")
	}
	recursive, err := searchBoolArgument(arg, "recursive", true)
	if err != nil {
		return "", err
	}
	caseSensitive, err := searchBoolArgument(arg, "case_sensitive", false)
	if err != nil {
		return "", err
	}
	maxResults, err := searchMaxResults(arg)
	if err != nil {
		return "", err
	}
	if !caseSensitive {
		pattern = strings.ToLower(pattern)
	}
	useGlob := strings.ContainsAny(pattern, "*?[")
	if useGlob {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return "", fmt.Errorf("参数 pattern 的通配符格式无效：%w", err)
		}
	}
	fsys, err := fileSystem()
	if err != nil {
		return "", err
	}

	result := struct {
		Path      string   `json:"path"`
		Matches   []string `json:"matches"`
		Truncated bool     `json:"truncated"`
	}{Path: path, Matches: []string{}}
	limitReached := errors.New("搜索结果达到数量限制")
	var search func(string) error
	search = func(directory string) error {
		entries, err := fsys.ReadDir(directory)
		if err != nil {
			return fmt.Errorf("搜索目录 %q 失败：%w", directory, err)
		}
		for _, entry := range entries {
			entryPath := filepath.Join(directory, entry.Name())
			if entry.IsDir() {
				if recursive {
					if err := search(entryPath); err != nil {
						return err
					}
				}
				continue
			}
			// 只返回普通文件，避免把目录链接、设备或管道当作文件。
			if !entry.Type().IsRegular() {
				continue
			}
			name := entry.Name()
			if !caseSensitive {
				name = strings.ToLower(name)
			}
			matched := strings.Contains(name, pattern)
			if useGlob {
				matched, err = filepath.Match(pattern, name)
				if err != nil {
					return err
				}
			}
			if !matched {
				continue
			}
			if len(result.Matches) == maxResults {
				result.Truncated = true
				return limitReached
			}
			result.Matches = append(result.Matches, entryPath)
		}
		return nil
	}
	if err := search(path); err != nil && !errors.Is(err, limitReached) {
		return "", err
	}
	sort.Strings(result.Matches)
	data, err := json.Marshal(result)
	return string(data), err
}

func searchBoolArgument(arg map[string]any, name string, defaultValue bool) (bool, error) {
	value, present := arg[name]
	if !present {
		return defaultValue, nil
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("参数 %s 必须是布尔值", name)
	}
	return flag, nil
}

func searchMaxResults(arg map[string]any) (int, error) {
	value, present := arg["max_results"]
	if !present {
		return 200, nil
	}
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case int:
		number = float64(value)
	default:
		return 0, errors.New("参数 max_results 必须是 1 至 10000 的整数")
	}
	if math.IsNaN(number) || number < 1 || number > 10000 || math.Trunc(number) != number {
		return 0, errors.New("参数 max_results 必须是 1 至 10000 的整数")
	}
	return int(number), nil
}

func (t SearchFiles) ToType() *APIImpl.Tool {
	return t.ToolMeta.ToType()
}
