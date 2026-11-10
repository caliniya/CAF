package FileToolInMainTool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type WriteFile struct {
	agenttool.ToolMeta
}

func (t *WriteFile) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID:             "WriteFile",
		Name:           "覆盖写入文件",
		DisplayType:    "info",
		DisplayContent: "arguments.content",
		Description:    "以 UTF-8 文本写入指定文件。均不传 start_line/end_line 时完整覆盖文件，不存在时创建，父文件夹必须已存在。传入任一行号时替换已有文件的指定行范围并保留其余内容，行号从 1 开始且包含结束行，范围越界时报错；末尾换行不额外计为空行。替换内容沿用选中范围的换行风格，必要时补齐行尾换行以保留后续行边界。content 可为空字符串，用于清空文件或删除选中的行。自动编辑权限下只能写入当前工作区。成功时返回 JSON，包含文件路径和实际写回文件的总字节数。",
		TimeoutParam:   "timeout_seconds",
		Permission:     agenttool.PermissionAutoEdit,
		Enabled:        true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       pathParameter(),
				"start_line": lineParameter("可选的起始行号，从 1 开始，省略时默认从第 1 行替换。必须是已有文件中的行。"),
				"end_line":   lineParameter("可选的结束行号，包含该行；省略时替换到文件末尾，不能小于 start_line，不能超出已有文件行数。"),
				"content": map[string]any{
					"type":        "string",
					"description": "要写入的文本。不传行号时为文件的完整新内容；传入行号时替换选中范围，可包含任意数量的行，空字符串删除选中的行。",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

func (t *WriteFile) Meta() *agenttool.ToolMeta {
	return &t.ToolMeta
}

func (t *WriteFile) Fun(ctx *agenttool.ToolContext, arg map[string]any) (string, error) {
	if ctx == nil || !ctx.Check(agenttool.PermissionAutoEdit) {
		if ctx != nil && ctx.PermissionLevel == agenttool.PermissionAutoEdit && ctx.WorkSpace == "" {
			return "", errors.New("未使用工作区，自动编辑权限下禁止写入文件")
		}
		return "", errors.New("没有写入文件所需的编辑权限")
	}
	path, err := resolveFilePath(ctx, arg)
	if err != nil {
		return "", err
	}
	content, ok := arg["content"].(string)
	if !ok {
		return "", errors.New("参数 content 必须是字符串")
	}
	lineRange, err := parseFileLineRange(arg)
	if err != nil {
		return "", err
	}

	var workspace, relativePath string
	if ctx.PermissionLevel == agenttool.PermissionAutoEdit {
		workspace, err = filepath.Abs(ctx.WorkSpace)
		if err != nil {
			return "", fmt.Errorf("解析工作区路径失败：%w", err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("解析文件路径失败：%w", err)
		}
		relativePath, err = filepath.Rel(workspace, path)
		if err != nil || !filepath.IsLocal(relativePath) {
			return "", errors.New("自动编辑权限下只能写入当前工作区内的文件")
		}
	}
	fsys, err := fileSystem()
	if err != nil {
		return "", err
	}
	info, err := fsys.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("检查目标文件失败：%w", err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return "", errors.New("目标路径必须是普通文件")
	}
	data := []byte(content)
	if lineRange.specified {
		original, err := fsys.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("读取待替换文件失败：%w", err)
		}
		data, err = lineRange.replace(original, content)
		if err != nil {
			return "", err
		}
	}
	if workspace != "" {
		// 在实际写入时约束路径，防止符号链接或目录替换绕过工作区边界。
		err = fsys.WriteFileInRoot(workspace, relativePath, data, 0o600)
	} else {
		err = fsys.WriteFile(path, data, 0o600)
	}
	if err != nil {
		return "", fmt.Errorf("写入文件失败：%w", err)
	}
	result, err := json.Marshal(struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
	}{Path: path, BytesWritten: len(data)})
	return string(result), err
}

func (t WriteFile) ToType() *APIImpl.Tool {
	return t.ToolMeta.ToType()
}
