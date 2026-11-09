package Tests

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	filetool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool/FileTool"
	backend "github.com/caliniya/caf/core/Backend"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
)

func useFileToolKit(t *testing.T) *filetool.FileTool {
	t.Helper()
	previous := backend.Platform
	t.Cleanup(func() { backend.Platform = previous })
	backend.Platform = &backend.Core{FS: &desktopBackend.DesktopFileSystem{}}
	return filetool.InitFileTool()
}

func writeFileToolFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileToolRegistration(t *testing.T) {
	kit := filetool.InitFileTool()
	want := map[string]agenttool.PermissionLevel{
		"ReadFile":      agenttool.PermissionReadOnly,
		"ListDirectory": agenttool.PermissionReadOnly,
		"SearchFiles":   agenttool.PermissionReadOnly,
		"WriteFile":     agenttool.PermissionAutoEdit,
	}
	if len(kit.List()) != len(want) || len(kit.ListName()) != len(want) {
		t.Fatalf("文件工具没有全部注册：%v", kit.ListName())
	}
	for id, permission := range want {
		tool := kit.ListName()[id]
		if tool == nil || tool.Meta().ID != id || !tool.Meta().Enabled || tool.Meta().Permission != permission {
			t.Fatalf("工具 %s 的注册或权限错误：%+v", id, tool)
		}
		declaration := tool.ToType()
		if declaration.Name != id {
			t.Fatalf("工具声明未使用 ID：%+v", declaration)
		}
		properties := declaration.Parameters["properties"].(map[string]any)
		if properties["path"] == nil || properties["timeout_seconds"] == nil {
			t.Fatalf("工具 %s 缺少路径或可选超时声明", id)
		}
	}
}

func TestFileToolListDirectory(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFileToolFixture(t, filepath.Join(workspace, "readme.txt"), "文本")
	writeFileToolFixture(t, filepath.Join(workspace, "nested", "hidden.txt"), "子目录")
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionReadOnly)
	output, err := kit.Tools["ListDirectory"].Fun(ctx, map[string]any{"path": []any{"."}})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Path    string
		Entries []struct{ Name, Path, Type string }
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path != workspace || len(result.Entries) != 2 {
		t.Fatalf("应只列出直接子项：%s", output)
	}
	for i, entry := range result.Entries {
		wantName, wantType := "nested", "directory"
		if i == 1 {
			wantName, wantType = "readme.txt", "file"
		}
		if entry.Name != wantName || entry.Type != wantType || entry.Path != filepath.Join(workspace, wantName) {
			t.Fatalf("目录项错误：%+v", entry)
		}
	}

	empty := t.TempDir()
	output, err = kit.Tools["ListDirectory"].Fun(nil, map[string]any{"path": []string{empty}})
	if err != nil {
		t.Fatal(err)
	}
	var emptyResult map[string]any
	if err := json.Unmarshal([]byte(output), &emptyResult); err != nil {
		t.Fatal(err)
	}
	if entries, ok := emptyResult["entries"].([]any); !ok || len(entries) != 0 {
		t.Fatalf("空目录应返回空数组：%s", output)
	}
	for _, path := range []string{filepath.Join(workspace, "readme.txt"), filepath.Join(workspace, "missing")} {
		if _, err := kit.Tools["ListDirectory"].Fun(ctx, map[string]any{"path": []string{path}}); err == nil {
			t.Fatalf("应拒绝非目录路径：%s", path)
		}
	}
}

func TestFileToolSearchFiles(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "folder.go"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.go", "README.TXT", "nested/beta.go", "nested/report.txt", "notes.md"} {
		writeFileToolFixture(t, filepath.Join(workspace, filepath.FromSlash(name)), "文件内容有 report，但搜索仅匹配文件名")
	}
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionReadOnly)
	cases := []struct {
		name      string
		options   map[string]any
		want      []string
		truncated bool
	}{
		{name: "recursive_glob", options: map[string]any{"pattern": "*.go"}, want: []string{
			filepath.Join(workspace, "alpha.go"), filepath.Join(workspace, "nested", "beta.go"),
		}},
		{name: "direct_only", options: map[string]any{"pattern": "*.go", "recursive": false}, want: []string{filepath.Join(workspace, "alpha.go")}},
		{name: "substring", options: map[string]any{"pattern": "report"}, want: []string{filepath.Join(workspace, "nested", "report.txt")}},
		{name: "case_insensitive", options: map[string]any{"pattern": "*.txt"}, want: []string{
			filepath.Join(workspace, "README.TXT"), filepath.Join(workspace, "nested", "report.txt"),
		}},
		{name: "case_sensitive", options: map[string]any{"pattern": "*.txt", "case_sensitive": true}, want: []string{filepath.Join(workspace, "nested", "report.txt")}},
		{name: "character_glob", options: map[string]any{"pattern": "[ab]*.go"}, want: []string{
			filepath.Join(workspace, "alpha.go"), filepath.Join(workspace, "nested", "beta.go"),
		}},
		{name: "truncated", options: map[string]any{"pattern": "*.go", "max_results": float64(1)}, want: []string{filepath.Join(workspace, "alpha.go")}, truncated: true},
		{name: "exact_limit", options: map[string]any{"pattern": "*.go", "max_results": float64(2)}, want: []string{
			filepath.Join(workspace, "alpha.go"), filepath.Join(workspace, "nested", "beta.go"),
		}},
		{name: "no_matches", options: map[string]any{"pattern": "missing"}, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.options["path"] = []any{"."}
			output, err := kit.Tools["SearchFiles"].Fun(ctx, tc.options)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Path      string
				Matches   []string
				Truncated bool
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if result.Path != workspace || !reflect.DeepEqual(result.Matches, tc.want) || result.Truncated != tc.truncated {
				t.Fatalf("搜索结果错误：%s，期望 %v，截断=%v", output, tc.want, tc.truncated)
			}
		})
	}

	for _, options := range []map[string]any{
		{},
		{"pattern": ""},
		{"pattern": "["},
		{"pattern": 1},
		{"pattern": "*", "recursive": "true"},
		{"pattern": "*", "case_sensitive": nil},
		{"pattern": "*", "max_results": 0},
		{"pattern": "*", "max_results": 1.5},
		{"pattern": "*", "max_results": math.NaN()},
		{"pattern": "*", "max_results": math.Inf(1)},
		{"pattern": "*", "max_results": 10001},
		{"pattern": "*", "max_results": "1"},
		{"pattern": "*", "path": []string{filepath.Join(workspace, "alpha.go")}},
		{"pattern": "*", "path": []string{filepath.Join(workspace, "missing")}},
	} {
		if _, present := options["path"]; !present {
			options["path"] = []string{"."}
		}
		if _, err := kit.Tools["SearchFiles"].Fun(ctx, options); err == nil {
			t.Fatalf("应拒绝无效搜索参数：%v", options)
		}
	}
}

func TestFileToolWriteFile(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	writeFileToolFixture(t, target, "很长的原始文件内容，覆盖后不应留下末尾")
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit)
	for _, content := range []string{"新内容\n", "", "再次写入"} {
		output, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": []any{"target.txt"}, "content": content})
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != content {
			t.Fatalf("覆盖写入没有完整替换旧内容：%q，错误=%v", data, err)
		}
		var result struct {
			Path         string `json:"path"`
			BytesWritten int    `json:"bytes_written"`
		}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatal(err)
		}
		if result.Path != target || result.BytesWritten != len([]byte(content)) {
			t.Fatalf("写入结果错误：%s", output)
		}
	}
	output, err := kit.Tools["ReadFile"].Fun(ctx, map[string]any{"path": []string{"target.txt"}})
	if err != nil || output != "再次写入" {
		t.Fatalf("应能读取覆盖后的内容：%q，错误=%v", output, err)
	}
	if _, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": []string{"new.txt"}, "content": "创建"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "new.txt"))
	if err != nil || string(data) != "创建" {
		t.Fatalf("创建文件失败：%q，错误=%v", data, err)
	}

	for _, arguments := range []map[string]any{
		{"path": []string{"target.txt"}},
		{"path": []string{"target.txt"}, "content": 123},
		{"path": []string{"."}, "content": "禁止写入目录"},
		{"path": []string{"missing", "new.txt"}, "content": "父目录不存在"},
	} {
		if _, err := kit.Tools["WriteFile"].Fun(ctx, arguments); err == nil {
			t.Fatalf("应拒绝无效写入参数：%v", arguments)
		}
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "再次写入" {
		t.Fatalf("失败调用修改了原文件：%q，错误=%v", data, err)
	}
}

func TestFileToolWritePermissions(t *testing.T) {
	kit := useFileToolKit(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "workspace-other")
	for _, directory := range []string{workspace, outside} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(workspace, "target.txt")
	external := filepath.Join(outside, "target.txt")
	writeFileToolFixture(t, target, "原文")
	writeFileToolFixture(t, external, "工作区外原文")
	cases := []struct {
		name string
		ctx  *agenttool.ToolContext
		path []string
	}{
		{name: "nil_context", path: []string{target}},
		{name: "read_only", ctx: agenttool.CreateContext(workspace, agenttool.PermissionReadOnly), path: []string{target}},
		{name: "no_workspace", ctx: agenttool.CreateContext("", agenttool.PermissionAutoEdit), path: []string{target}},
		{name: "parent_traversal", ctx: agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit), path: []string{"..", "workspace-other", "target.txt"}},
		{name: "absolute_outside", ctx: agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit), path: []string{external}},
		{name: "create_outside", ctx: agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit), path: []string{outside, "new.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := kit.Tools["WriteFile"].Fun(tc.ctx, map[string]any{"path": tc.path, "content": "不应写入"}); err == nil {
				t.Fatal("应拒绝没有权限或工作区外的写入")
			}
		})
	}
	for path, content := range map[string]string{target: "原文", external: "工作区外原文"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != content {
			t.Fatalf("无权限的调用修改了 %s：%q，错误=%v", path, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("不应在工作区外创建文件：%v", err)
	}
	for _, permission := range []agenttool.PermissionLevel{agenttool.PermissionAutoApprove, agenttool.PermissionFullAccess} {
		ctx := agenttool.CreateContext("", permission)
		if _, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": []string{external}, "content": "允许写入"}); err != nil {
			t.Fatalf("权限 %d 应允许绝对路径写入：%v", permission, err)
		}
	}
}

func TestFileToolSharedPathValidation(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit)
	for id, tool := range kit.Tools {
		t.Run(id, func(t *testing.T) {
			for _, path := range []any{nil, "file.txt", []any{}, []string{}, []any{1}, []any{" "}, []string{".", ""}} {
				if _, err := tool.Fun(ctx, map[string]any{"path": path, "pattern": "*", "content": "文本"}); err == nil {
					t.Fatalf("应拒绝无效路径参数：%v", path)
				}
			}
			if _, err := tool.Fun(ctx, map[string]any{"pattern": "*", "content": "文本"}); err == nil {
				t.Fatal("应拒绝缺少路径参数")
			}
			if _, err := tool.Fun(agenttool.CreateContext("", agenttool.PermissionFullAccess), map[string]any{"path": []string{"relative.txt"}, "pattern": "*", "content": "文本"}); err == nil {
				t.Fatal("没有工作区时应拒绝相对路径")
			}
		})
	}
}

func TestFileToolSymlinks(t *testing.T) {
	kit := useFileToolKit(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	for _, directory := range []string{workspace, outside} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(outside, "keep.txt")
	writeFileToolFixture(t, external, "保持原内容")
	writeFileToolFixture(t, filepath.Join(workspace, "real.txt"), "工作区内")
	if err := os.Symlink(filepath.Join("..", "outside"), filepath.Join(workspace, "linked")); err != nil {
		t.Skipf("当前环境不允许创建符号链接：%v", err)
	}
	if err := os.Symlink(filepath.Join("..", "outside", "keep.txt"), filepath.Join(workspace, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit)
	for _, path := range [][]string{{"linked", "keep.txt"}, {"linked", "new.txt"}, {"linked.txt"}} {
		if _, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": path, "content": "禁止越界"}); err == nil {
			t.Fatalf("应拒绝符号链接越界写入：%v", path)
		}
	}
	data, err := os.ReadFile(external)
	if err != nil || string(data) != "保持原内容" {
		t.Fatalf("符号链接目标被修改：%q，错误=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("不应通过符号链接创建工作区外文件：%v", err)
	}
	output, err := kit.Tools["SearchFiles"].Fun(ctx, map[string]any{"path": []string{"."}, "pattern": "*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Matches []string }
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Matches, []string{filepath.Join(workspace, "real.txt")}) {
		t.Fatalf("搜索不应跟随符号链接：%s", output)
	}
	if err := os.Symlink("real.txt", filepath.Join(workspace, "internal.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": []string{"internal.txt"}, "content": "工作区内链接可写"}); err != nil {
		t.Fatalf("应允许指向工作区内文件的相对符号链接：%v", err)
	}
}

func TestFileToolUninitializedFileSystem(t *testing.T) {
	previous := backend.Platform
	t.Cleanup(func() { backend.Platform = previous })
	kit := filetool.InitFileTool()
	for _, instance := range []*backend.Core{nil, {}} {
		backend.Platform = instance
		for id, tool := range kit.Tools {
			if _, err := tool.Fun(agenttool.CreateContext(t.TempDir(), agenttool.PermissionFullAccess), map[string]any{
				"path": []string{"."}, "pattern": "*", "content": "文本",
			}); err == nil {
				t.Fatalf("文件系统未初始化时工具 %s 应返回错误", id)
			}
		}
	}
}
