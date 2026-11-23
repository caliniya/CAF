package Tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	filetool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool/FileTool"
	platform "github.com/caliniya/caf/platform"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
)

func assertRecoveredFile(t *testing.T, path string, data []byte, info fs.FileInfo) {
	t.Helper()
	if info == nil {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("file should not exist: %s / %v", path, err)
		}
		return
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("file bytes changed: %q, want %q / %v", actual, data, err)
	}
	current, err := os.Stat(path)
	if err != nil || current.Mode().Perm() != info.Mode().Perm() || !current.ModTime().Equal(info.ModTime()) {
		t.Fatalf("file attributes changed: %+v, want %+v / %v", current, info, err)
	}
}

func TestFileToolRecoveryRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name, tool, content string
		before              []byte
		exists, byLine      bool
	}{
		{name: "overwrite_binary_original", tool: "WriteFile", exists: true, before: []byte{0, 255, '\r', '\n', 128}, content: "新内容\n"},
		{name: "replace_crlf_lines", tool: "WriteFile", exists: true, before: []byte("第一行\r\nsecond\r\n第三行\r\n"), content: "替换", byLine: true},
		{name: "empty_file", tool: "WriteFile", exists: true, before: []byte{}, content: "nonempty"},
		{name: "create_file", tool: "WriteFile", content: "created"},
		{name: "delete_binary", tool: "DeleteFile", exists: true, before: []byte{0, 255, 128, '\r', '\n'}},
		{name: "delete_empty", tool: "DeleteFile", exists: true, before: []byte{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "note.txt")
			var beforeInfo fs.FileInfo
			if test.exists {
				if err := os.WriteFile(path, test.before, 0o640); err != nil {
					t.Fatal(err)
				}
				stamp := time.Unix(1700000000, 123456700)
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
				beforeInfo, _ = os.Stat(path)
			}
			ctx := fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
			ctx.FileSystem = &desktop.DesktopFileSystem{}
			args := map[string]any{"path": []string{"note.txt"}, "content": test.content}
			if test.byLine {
				args["start_line"], args["end_line"] = 2, 2
			}
			tool := filetool.InitFileTool().Tools[test.tool]
			output, err := tool.Fun(ctx, args)
			if err != nil || !json.Valid([]byte(output)) {
				t.Fatalf("tool failed: %s / %v", output, err)
			}
			var after []byte
			var afterInfo fs.FileInfo
			if test.tool == "WriteFile" {
				after, _ = os.ReadFile(path)
				afterInfo, _ = os.Stat(path)
			}
			// 新工具实例只靠恢复文件即可撤回和重放，不依赖执行时的内存。
			reversible := filetool.InitFileTool().Tools[test.tool].(agenttool.ReversibleTool)
			for i := 0; i < 2; i++ {
				if err := reversible.Restore(ctx, ctx.RecoveryFile); err != nil {
					t.Fatal(err)
				}
				assertRecoveredFile(t, path, test.before, beforeInfo)
			}
			for i := 0; i < 2; i++ {
				if err := reversible.Reapply(ctx, ctx.RecoveryFile); err != nil {
					t.Fatal(err)
				}
				assertRecoveredFile(t, path, after, afterInfo)
			}
		})
	}
}

type failingFileRecoveryFS struct {
	platform.FilePlatform
	cacheWrite, cacheMove, failWrite, failMove int
	partialWrite, removedWithError             bool
	cancel                                     context.CancelFunc
}

func (f *failingFileRecoveryFS) WriteFileInRoot(root, name string, data []byte, mode fs.FileMode) error {
	if strings.HasSuffix(name, ".json.tmp") {
		f.cacheWrite++
		if f.cacheWrite == f.failWrite {
			return errors.New("injected recovery write failure")
		}
	} else if name == "note.txt" {
		if f.partialWrite {
			if err := f.FilePlatform.WriteFileInRoot(root, name, []byte("partial"), mode); err != nil {
				return err
			}
			return errors.New("injected partial file write")
		}
		if f.cancel != nil {
			if err := f.FilePlatform.WriteFileInRoot(root, name, data, mode); err != nil {
				return err
			}
			f.cancel()
			return nil
		}
	}
	return f.FilePlatform.WriteFileInRoot(root, name, data, mode)
}

func (f *failingFileRecoveryFS) MoveFile(source, target string) error {
	f.cacheMove++
	if f.cacheMove == f.failMove {
		return errors.New("injected recovery commit failure")
	}
	return f.FilePlatform.MoveFile(source, target)
}

func (f *failingFileRecoveryFS) DeleteFileInRoot(root, name string) error {
	err := f.FilePlatform.DeleteFileInRoot(root, name)
	if err == nil && name == "note.txt" {
		if f.cancel != nil {
			f.cancel()
		}
		if f.removedWithError {
			return errors.New("injected delete failure after removal")
		}
	}
	return err
}

func TestFileToolRecoveryFailureKeepsOriginal(t *testing.T) {
	for _, test := range []struct {
		name, tool             string
		failWrite, failMove    int
		partialWrite, canceled bool
		removedWithError       bool
	}{
		{name: "initial_cache_write", tool: "WriteFile", failWrite: 1},
		{name: "initial_cache_commit", tool: "WriteFile", failMove: 1},
		{name: "final_cache_write", tool: "WriteFile", failWrite: 2},
		{name: "final_cache_commit", tool: "WriteFile", failMove: 2},
		{name: "partial_target_write", tool: "WriteFile", partialWrite: true},
		{name: "cancel_after_write", tool: "WriteFile", canceled: true},
		{name: "delete_cache_write", tool: "DeleteFile", failWrite: 1},
		{name: "delete_cache_commit", tool: "DeleteFile", failMove: 1},
		{name: "cancel_after_delete", tool: "DeleteFile", canceled: true},
		{name: "delete_returns_error_after_removal", tool: "DeleteFile", removedWithError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "note.txt")
			writeFileToolFixture(t, path, "original\r\n")
			stamp := time.Unix(1700000000, 0)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(path)
			ctx := fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx.Context = parent
			fsys := &failingFileRecoveryFS{FilePlatform: &desktop.DesktopFileSystem{}, failWrite: test.failWrite, failMove: test.failMove, partialWrite: test.partialWrite, removedWithError: test.removedWithError}
			if test.canceled {
				fsys.cancel = cancel
			}
			ctx.FileSystem = fsys
			tool := filetool.InitFileTool().Tools[test.tool]
			if _, err := tool.Fun(ctx, map[string]any{"path": []string{"note.txt"}, "content": "replacement"}); err == nil {
				t.Fatal("injected failure did not fail the tool")
			}
			assertRecoveredFile(t, path, []byte("original\r\n"), info)
			if _, err := os.Stat(ctx.RecoveryFile); err == nil {
				ctx.Context = context.Background()
				if err := tool.(agenttool.ReversibleTool).Restore(ctx, ctx.RecoveryFile); err != nil {
					t.Fatalf("recovery copy should remain usable: %v", err)
				}
				assertRecoveredFile(t, path, []byte("original\r\n"), info)
			}
		})
	}
}

func TestFileToolRecoveryRequiresBackupAndValidIdentity(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	writeFileToolFixture(t, path, "original")
	kit := filetool.InitFileTool()
	ctx := fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
	ctx.FileSystem = &desktop.DesktopFileSystem{}
	recoveryPath := ctx.RecoveryFile
	for _, invalidPath := range []string{"", "relative.json"} {
		ctx.RecoveryFile = invalidPath
		for _, id := range []string{"WriteFile", "DeleteFile"} {
			if _, err := kit.Tools[id].Fun(ctx, map[string]any{"path": []string{"note.txt"}, "content": "replacement"}); err == nil {
				t.Fatal("file mutation must require an absolute recovery file")
			}
		}
	}
	ctx.RecoveryFile = recoveryPath
	if _, err := kit.Tools["WriteFile"].Fun(ctx, map[string]any{"path": []string{"note.txt"}, "content": "replacement"}); err != nil {
		t.Fatal(err)
	}
	// 宿主以完全访问权限恢复时，仍不能绕过副本保存的工作区边界。
	ctx.PermissionLevel = agenttool.PermissionFullAccess
	var record map[string]any
	body, _ := os.ReadFile(recoveryPath)
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeFileToolFixture(t, outside, "outside-original")
	for _, field := range []string{"toolId", "callId", "workspaceId", "workspace", "path", "before"} {
		t.Run(field, func(t *testing.T) {
			var changed map[string]any
			_ = json.Unmarshal(body, &changed)
			changed[field] = "invalid"
			if field == "path" {
				changed[field] = outside
			}
			if field == "before" {
				changed[field] = map[string]any{"exists": true, "data": "", "mode": 0, "modified": "0001-01-01T00:00:00Z"}
			}
			data, _ := json.Marshal(changed)
			if err := os.WriteFile(recoveryPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			for _, reapply := range []bool{false, true} {
				tool := kit.Tools["WriteFile"].(agenttool.ReversibleTool)
				var err error
				if reapply {
					err = tool.Reapply(ctx, recoveryPath)
				} else {
					err = tool.Restore(ctx, recoveryPath)
				}
				if err == nil {
					t.Fatal("invalid recovery must be rejected before changing files")
				}
			}
			if data, _ := os.ReadFile(path); string(data) != "replacement" {
				t.Fatal("invalid recovery changed target")
			}
			if data, _ := os.ReadFile(outside); string(data) != "outside-original" {
				t.Fatal("invalid recovery escaped its original workspace")
			}
		})
	}
}

func TestDeleteFileBoundariesAndSymlinks(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	external := filepath.Join(outside, "outside.txt")
	writeFileToolFixture(t, path, "inside")
	writeFileToolFixture(t, external, "outside")
	directory := filepath.Join(workspace, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filetool.InitFileTool().Tools["DeleteFile"]
	for _, test := range []struct {
		permission agenttool.PermissionLevel
		workspace  string
		path       string
	}{
		{agenttool.PermissionReadOnly, workspace, path},
		{agenttool.PermissionAutoEdit, "", path},
		{agenttool.PermissionAutoEdit, workspace, external},
		{agenttool.PermissionFullAccess, workspace, directory},
		{agenttool.PermissionFullAccess, workspace, filepath.Join(workspace, "missing")},
	} {
		ctx := fileToolContext(t, test.workspace, test.permission)
		ctx.FileSystem = &desktop.DesktopFileSystem{}
		if _, err := tool.Fun(ctx, map[string]any{"path": []string{test.path}}); err == nil {
			t.Fatalf("delete should reject this target: %+v", test)
		}
	}
	for _, name := range []string{"internal-link.txt", "external-link.txt"} {
		target := path
		if name == "external-link.txt" {
			target = external
		}
		link := filepath.Join(workspace, name)
		if err := os.Symlink(target, link); err != nil {
			t.Logf("symlink unavailable on this host: %v", err)
			continue
		}
		ctx := fileToolContext(t, workspace, agenttool.PermissionFullAccess)
		ctx.FileSystem = &desktop.DesktopFileSystem{}
		if _, err := tool.Fun(ctx, map[string]any{"path": []string{link}}); err == nil {
			t.Fatal("delete should reject file symlinks")
		}
		if actual, err := os.Readlink(link); err != nil || actual != target {
			t.Fatalf("rejected link was changed: %s / %v", actual, err)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != "inside" {
		t.Fatal("rejected deletion changed the workspace file")
	}
	if data, _ := os.ReadFile(external); string(data) != "outside" {
		t.Fatal("rejected deletion changed an outside file")
	}
}

func TestWriteFileRecoverySupportsInternalLinksAndRejectsDanglingLinks(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	link := filepath.Join(workspace, "linked.txt")
	writeFileToolFixture(t, path, "original")
	if err := os.Symlink("note.txt", link); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	ctx := fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
	ctx.FileSystem = &desktop.DesktopFileSystem{}
	tool := filetool.InitFileTool().Tools["WriteFile"]
	if _, err := tool.Fun(ctx, map[string]any{"path": []string{"linked.txt"}, "content": "changed"}); err != nil {
		t.Fatal(err)
	}
	for _, reapply := range []bool{false, true, false} {
		var err error
		if reapply {
			err = tool.(agenttool.ReversibleTool).Reapply(ctx, ctx.RecoveryFile)
		} else {
			err = tool.(agenttool.ReversibleTool).Restore(ctx, ctx.RecoveryFile)
		}
		if err != nil {
			t.Fatal(err)
		}
		want := "original"
		if reapply {
			want = "changed"
		}
		assertRecoveryContent(t, path, want)
		if target, err := os.Readlink(link); err != nil || target != "note.txt" {
			t.Fatal("restoration changed the existing file symlink", target, err)
		}
	}
	dangling := filepath.Join(workspace, "dangling.txt")
	if err := os.Symlink("missing.txt", dangling); err != nil {
		t.Fatal(err)
	}
	ctx = fileToolContext(t, workspace, agenttool.PermissionAutoEdit)
	ctx.FileSystem = &desktop.DesktopFileSystem{}
	if _, err := tool.Fun(ctx, map[string]any{"path": []string{"dangling.txt"}, "content": "must-not-create"}); err == nil {
		t.Fatal("dangling file symlink has no recoverable original state")
	}
	if target, err := os.Readlink(dangling); err != nil || target != "missing.txt" {
		t.Fatal("rejected dangling link was modified", target, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "missing.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("rejected link write created a file", err)
	}
}
