package Tests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	filetool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool/FileTool"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
)

func TestFileToolChangeSummariesUseSavedSnapshots(t *testing.T) {
	for _, test := range []struct {
		name, tool, before, after string
		exists, byLine, binary    bool
		action                    string
		added, removed            int
	}{
		{name: "unchanged", tool: "WriteFile", exists: true, before: "same\n", after: "same\n", action: "modify"},
		{name: "two_replacements_preserve_middle", tool: "WriteFile", exists: true, before: "one\nold\nkeep\nold-end\nlast\n", after: "one\nnew\nkeep\nnew-end\nlast\n", action: "modify", added: 2, removed: 2},
		{name: "repeated_line_insertion", tool: "WriteFile", exists: true, before: "a\nb\na\nend\n", after: "a\na\nb\na\nend\n", action: "modify", added: 1},
		{name: "line_range_crlf", tool: "WriteFile", exists: true, before: "第一行\r\nold\r\n尾行\r\n", after: "替换", byLine: true, action: "modify", added: 1, removed: 1},
		{name: "create", tool: "WriteFile", after: "one\ntwo\n", action: "create", added: 2},
		{name: "create_empty", tool: "WriteFile", action: "create"},
		{name: "clear", tool: "WriteFile", exists: true, before: "one\ntwo", action: "modify", removed: 2},
		{name: "change_line_ending", tool: "WriteFile", exists: true, before: "one\r\n", after: "one\n", action: "modify", added: 1, removed: 1},
		{name: "delete_text", tool: "DeleteFile", exists: true, before: "one\ntwo\n", action: "delete", removed: 2},
		{name: "delete_empty", tool: "DeleteFile", exists: true, action: "delete"},
		{name: "delete_binary", tool: "DeleteFile", exists: true, before: string([]byte{0, 255, '\n', 128}), action: "delete", binary: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			file := filepath.Join(workspace, "test.txt")
			if test.exists {
				writeFileToolFixture(t, file, test.before)
			}
			ctx := fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
			ctx.FileSystem = &desktop.DesktopFileSystem{}
			args := map[string]any{"path": []string{"test.txt"}, "content": test.after}
			if test.byLine {
				args["start_line"], args["end_line"] = 2, 2
			}
			tool := filetool.InitFileTool().Tools[test.tool]
			if _, err := tool.Fun(ctx, args); err != nil {
				t.Fatal(err)
			}
			var after []byte
			if test.tool == "WriteFile" {
				after, _ = os.ReadFile(file)
			}
			// 读取摘要时文件已被外部改动；摘要必须描述工具实际执行的变更。
			writeFileToolFixture(t, file, "unrelated external state")
			reversible := filetool.InitFileTool().Tools[test.tool].(agenttool.ReversibleTool)
			changes, err := reversible.Changes(ctx, ctx.RecoveryFile)
			if err != nil || len(changes) != 1 {
				t.Fatal("summary failed", changes, err)
			}
			change := changes[0]
			if change.Target != file || change.Action != test.action || change.Binary != test.binary || change.Approximate ||
				change.AddedLines != test.added || change.RemovedLines != test.removed ||
				change.BeforeBytes != len(test.before) || change.AfterBytes != len(after) {
				t.Fatalf("incorrect actual change summary: %+v", change)
			}
			assertRecoveryContent(t, file, "unrelated external state")
			if _, err := reversible.Changes(nil, ctx.RecoveryFile); err == nil {
				t.Fatal("summary accepted an unavailable context")
			}
			ctx.CallID = "different-call"
			if _, err := reversible.Changes(ctx, ctx.RecoveryFile); err == nil {
				t.Fatal("summary accepted a different tool call")
			}
		})
	}
}

func TestFileChangeSummaryBoundsLargeDiffWork(t *testing.T) {
	var before, after bytes.Buffer
	for i := 0; i < 2400; i++ {
		if i == 1200 {
			before.WriteString("shared-middle\n")
			after.WriteString("shared-middle\n")
		} else {
			fmt.Fprintf(&before, "old-%d\n", i)
			fmt.Fprintf(&after, "new-%d\n", i)
		}
	}
	workspace := t.TempDir()
	writeFileToolFixture(t, filepath.Join(workspace, "large.txt"), before.String())
	ctx := fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
	ctx.FileSystem = &desktop.DesktopFileSystem{}
	tool := filetool.InitFileTool().Tools["WriteFile"].(agenttool.ReversibleTool)
	if _, err := tool.Fun(ctx, map[string]any{"path": []string{"large.txt"}, "content": after.String()}); err != nil {
		t.Fatal(err)
	}
	changes, err := tool.Changes(ctx, ctx.RecoveryFile)
	if err != nil || len(changes) != 1 {
		t.Fatal(err, changes)
	}
	change := changes[0]
	if !change.Approximate || change.AddedLines != 2400 || change.RemovedLines != 2400 || change.BeforeBytes != before.Len() || change.AfterBytes != after.Len() {
		t.Fatal("expensive diff should report an explicit estimate with exact byte counts", change)
	}
}
