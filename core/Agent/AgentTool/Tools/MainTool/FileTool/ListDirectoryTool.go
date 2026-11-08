package FileToolInMainTool

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type ListDirectory struct {
	agenttool.ToolMeta
}

func (t *ListDirectory) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID:           "ListDirectory",
		Name:         "列出目录",
		Description:  "列出指定文件夹的直接子项，不递归。返回 JSON，包含名称、路径和类型（file、directory、symlink 或 other）。",
		TimeoutParam: "timeout_seconds",
		Permission:   agenttool.PermissionReadOnly,
		Enabled:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": pathParameter(),
			},
			"required": []string{"path"},
		},
	}
}

func (t *ListDirectory) Meta() *agenttool.ToolMeta {
	return &t.ToolMeta
}

func (t *ListDirectory) Fun(ctx *agenttool.ToolContext, arg map[string]any) (string, error) {
	path, err := resolveFilePath(ctx, arg)
	if err != nil {
		return "", err
	}
	fsys, err := fileSystem()
	if err != nil {
		return "", err
	}
	entries, err := fsys.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("列出目录失败：%w", err)
	}

	type directoryEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	result := struct {
		Path    string           `json:"path"`
		Entries []directoryEntry `json:"entries"`
	}{Path: path, Entries: make([]directoryEntry, 0, len(entries))}
	for _, entry := range entries {
		kind := "file"
		switch {
		case entry.IsDir():
			kind = "directory"
		case entry.Type()&fs.ModeSymlink != 0:
			kind = "symlink"
		case !entry.Type().IsRegular():
			kind = "other"
		}
		result.Entries = append(result.Entries, directoryEntry{
			Name: entry.Name(), Path: filepath.Join(path, entry.Name()), Type: kind,
		})
	}
	data, err := json.Marshal(result)
	return string(data), err
}

func (t ListDirectory) ToType() *APIImpl.Tool {
	return t.ToolMeta.ToType()
}
