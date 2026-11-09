package Tests

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

func TestFileToolReadFileLines(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionReadOnly)
	for _, tc := range []struct {
		name, original, want string
		options              map[string]any
	}{
		{name: "whole_file", original: "一\r\n\r\n三\r\n", want: "一\r\n\r\n三\r\n"},
		{name: "inclusive_range", original: "一\n二\n三\n四", options: map[string]any{"start_line": float64(2), "end_line": float64(3)}, want: "二\n三\n"},
		{name: "start_only", original: "一\n二\n三", options: map[string]any{"start_line": 2}, want: "二\n三"},
		{name: "end_only", original: "一\n二\n三", options: map[string]any{"end_line": 2}, want: "一\n二\n"},
		{name: "single_line", original: "一\n二\n三", options: map[string]any{"start_line": 2, "end_line": 2}, want: "二\n"},
		{name: "crlf_and_blank", original: "一\r\n\r\n三\r\n", options: map[string]any{"start_line": 2, "end_line": 3}, want: "\r\n三\r\n"},
		{name: "mixed_newlines", original: "一\r\n二\n三", options: map[string]any{"end_line": 2}, want: "一\r\n二\n"},
		{name: "end_past_eof", original: "一\n二", options: map[string]any{"start_line": 2, "end_line": 20}, want: "二"},
		{name: "start_past_eof", original: "一\n二\n", options: map[string]any{"start_line": 3}, want: ""},
		{name: "empty_file", options: map[string]any{"start_line": 1}, want: ""},
		{name: "one_blank_line", original: "\n", options: map[string]any{"start_line": 1, "end_line": 1}, want: "\n"},
		{name: "long_line", original: "前\n" + strings.Repeat("长", 70000) + "\n后", options: map[string]any{"start_line": 2, "end_line": 2}, want: strings.Repeat("长", 70000) + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFileToolFixture(t, filepath.Join(workspace, "target.txt"), tc.original)
			arguments := map[string]any{"path": []string{"target.txt"}}
			for key, value := range tc.options {
				arguments[key] = value
			}
			output, err := kit.Tools["ReadFile"].Fun(ctx, arguments)
			if err != nil || output != tc.want {
				t.Fatalf("按行读取结果错误：长度=%d，期望长度=%d，错误=%v", len(output), len(tc.want), err)
			}
		})
	}
}

func TestFileToolWriteFileLines(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit)
	for _, tc := range []struct {
		name, original, content, want string
		options                       map[string]any
	}{
		{name: "single_line", original: "一\n二\n三", content: "新", options: map[string]any{"start_line": float64(2), "end_line": float64(2)}, want: "一\n新\n三"},
		{name: "inclusive_range", original: "一\n二\n三\n四", content: "新", options: map[string]any{"start_line": 2, "end_line": 3}, want: "一\n新\n四"},
		{name: "more_lines", original: "一\n二\n三", content: "甲\n乙\n", options: map[string]any{"start_line": 2, "end_line": 2}, want: "一\n甲\n乙\n三"},
		{name: "start_only", original: "一\n二\n三", content: "新", options: map[string]any{"start_line": 2}, want: "一\n新"},
		{name: "end_only", original: "一\n二\n三", content: "新", options: map[string]any{"end_line": 2}, want: "新\n三"},
		{name: "delete_lines", original: "一\n二\n三\n四", content: "", options: map[string]any{"start_line": 2, "end_line": 3}, want: "一\n四"},
		{name: "delete_all", original: "一\n二\n", content: "", options: map[string]any{"start_line": 1}, want: ""},
		{name: "preserve_eof_newline", original: "一\n二\n", content: "新", options: map[string]any{"start_line": 2}, want: "一\n新\n"},
		{name: "preserve_no_eof_newline", original: "一\n二", content: "新", options: map[string]any{"start_line": 2}, want: "一\n新"},
		{name: "explicit_eof_newline", original: "一\n二", content: "新\n", options: map[string]any{"start_line": 2}, want: "一\n新\n"},
		{name: "crlf", original: "一\r\n二\r\n三\r\n", content: "甲\n乙", options: map[string]any{"start_line": 2, "end_line": 2}, want: "一\r\n甲\r\n乙\r\n三\r\n"},
		{name: "crlf_content", original: "一\r\n二\r\n三", content: "新\r\n", options: map[string]any{"start_line": 2, "end_line": 2}, want: "一\r\n新\r\n三"},
		{name: "crlf_unterminated_last_line", original: "一\r\n二", content: "甲\n乙", options: map[string]any{"start_line": 2}, want: "一\r\n甲\r\n乙"},
		{name: "lf_content", original: "一\n二\n三", content: "新\r\n", options: map[string]any{"start_line": 2, "end_line": 2}, want: "一\n新\n三"},
		{name: "replace_blank_line", original: "一\n\n三", content: "新", options: map[string]any{"start_line": 2, "end_line": 2}, want: "一\n新\n三"},
		{name: "replace_one_blank_line", original: "\n", content: "新", options: map[string]any{"start_line": 1, "end_line": 1}, want: "新\n"},
		{name: "preserve_mixed_newlines", original: "一\r\n二\n三\r\n", content: "新", options: map[string]any{"start_line": 2, "end_line": 2}, want: "一\r\n新\n三\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFileToolFixture(t, target, tc.original)
			arguments := map[string]any{"path": []string{"target.txt"}, "content": tc.content}
			for key, value := range tc.options {
				arguments[key] = value
			}
			output, err := kit.Tools["WriteFile"].Fun(ctx, arguments)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != tc.want {
				t.Fatalf("按行写入结果错误：%q，期望=%q，错误=%v", data, tc.want, err)
			}
			var result struct {
				Path         string `json:"path"`
				BytesWritten int    `json:"bytes_written"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if result.Path != target || result.BytesWritten != len(data) {
				t.Fatalf("写入结果错误：%s", output)
			}
		})
	}
}

func TestFileToolLineValidation(t *testing.T) {
	kit := useFileToolKit(t)
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	const original = "一\n二\n"
	writeFileToolFixture(t, target, original)
	ctx := agenttool.CreateContext(workspace, agenttool.PermissionAutoEdit)
	for _, id := range []string{"ReadFile", "WriteFile"} {
		t.Run(id, func(t *testing.T) {
			for _, name := range []string{"start_line", "end_line"} {
				for _, value := range []any{nil, 0, -1, 1.5, "2", true, math.NaN(), math.Inf(1), math.Inf(-1), float64(int(^uint(0) >> 1))} {
					if _, err := kit.Tools[id].Fun(ctx, map[string]any{"path": []string{"target.txt"}, "content": "错误写入", name: value}); err == nil {
						t.Fatalf("应拒绝无效行号：%s=%v", name, value)
					}
				}
			}
			if _, err := kit.Tools[id].Fun(ctx, map[string]any{"path": []string{"target.txt"}, "content": "错误写入", "start_line": 2, "end_line": 1}); err == nil {
				t.Fatal("应拒绝结束行小于起始行")
			}
		})
	}
	for _, options := range []map[string]any{{"start_line": 3}, {"end_line": 3}, {"start_line": 2, "end_line": 10}} {
		options["path"] = []string{"target.txt"}
		options["content"] = "错误写入"
		if _, err := kit.Tools["WriteFile"].Fun(ctx, options); err == nil {
			t.Fatalf("应拒绝越界写入：%v", options)
		}
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != original {
		t.Fatalf("失败调用修改了原文件：%q，错误=%v", data, err)
	}
	writeFileToolFixture(t, filepath.Join(workspace, "empty.txt"), "")
	for _, name := range []string{"empty.txt", "missing.txt"} {
		if _, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": []string{name}, "content": "错误写入", "start_line": 1}); err == nil {
			t.Fatalf("应拒绝对没有行的文件按行写入：%s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "missing.txt")); !os.IsNotExist(err) {
		t.Fatalf("按行写入不应创建文件：%v", err)
	}
	data, err = os.ReadFile(filepath.Join(workspace, "empty.txt"))
	if err != nil || len(data) != 0 {
		t.Fatalf("失败调用修改了空文件：%q，错误=%v", data, err)
	}
}
