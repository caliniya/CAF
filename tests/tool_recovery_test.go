package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	platform "github.com/caliniya/caf/platform"
	identity "github.com/caliniya/caf/utils/id"
)

func recoveryChat(t *testing.T, core *backend.Core) (*chat.ChatTask, string) {
	t.Helper()
	workspace, err := core.CreateWorkspace("Recovery project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	task, err = core.SetChatWorkspace(task.ID, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolConfig(task.ID, `{"permission":2}`); err != nil {
		t.Fatal(err)
	}
	return task, workspace.Path
}

func recoveryOperations(t *testing.T, core *backend.Core, chatID string) (*chat.ChatTask, []agenttool.ToolOperation) {
	t.Helper()
	task, err := core.GetChat(chatID)
	if err != nil {
		t.Fatal(err)
	}
	path, err := task.Tree.CurrentPath()
	if err != nil {
		t.Fatal(err)
	}
	var operations []agenttool.ToolOperation
	for _, node := range path {
		operations = append(operations, node.ToolOperations...)
	}
	return task, operations
}

func recoveryCachePath(core *backend.Core, operation agenttool.ToolOperation) string {
	root := core.Paths.InternalWorkspaces
	if root == "" {
		root = filepath.Join(core.Paths.Data, "InternalWorkspaces")
	}
	folder := operation.WorkspaceID
	if folder == "" {
		folder = "unscoped"
	}
	return filepath.Join(root, "WorkspaceCache", folder, operation.ID+".json")
}

func assertRecoveryContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("wrong recovered content: %q, want %q / %v", data, want, err)
	}
}

func TestToolRecoveryAcrossRepeatedCallsBranchesAndRestart(t *testing.T) {
	core, _ := approvalCore(t)
	core.Paths.InternalWorkspaces = filepath.Join(core.Paths.Data, "custom-internal")
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "baseline\r\n")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("provider-reused", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "first"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{
				approvalCall("provider-reused", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "second"}),
				approvalCall("delete", "DeleteFile", map[string]any{"path": []string{"note.txt"}}),
			}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			assertRecoveryContent(t, file, "baseline\r\n")
			return []api.ToolCall{approvalCall("provider-reused", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "alternate"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "write first", ""); err != nil {
		t.Fatal(err)
	}
	first, _ := recoveryOperations(t, core, task.ID)
	firstPath, _ := first.Tree.CurrentPath()
	userID := firstPath[0].ID
	firstReply := firstPath[1].ID
	if err := core.SendChatMessage(task.ID, "write then delete", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("DeleteFile was not executed", err)
	}
	_, operations := recoveryOperations(t, core, task.ID)
	if len(operations) != 3 || operations[0].ID == operations[1].ID || operations[0].CallID != operations[1].CallID {
		t.Fatalf("provider call IDs must not cause recovery file collisions: %+v", operations)
	}
	for _, operation := range operations {
		if !identity.Valid(operation.ID, identity.Recovery) || operation.Pending || operation.WorkspaceID != task.WorkspaceID || operation.Workspace != project {
			t.Fatalf("invalid persisted operation: %+v", operation)
		}
		var record struct{ ToolID, CallID, WorkspaceID, Workspace string }
		memoryReadJSON(t, recoveryCachePath(core, operation), &record)
		if record.ToolID != operation.ToolID || record.CallID != operation.CallID || record.WorkspaceID != task.WorkspaceID || record.Workspace != project {
			t.Fatalf("recovery identity was not preserved: %+v", record)
		}
	}
	if err := core.RegenerateChatMessage(task.ID, firstReply, ""); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "alternate")
	if _, err := core.SetChatBranch(task.ID, userID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("old branch should reapply its final deletion", err)
	}
	if _, err := core.SetChatBranch(task.ID, userID, 1); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "alternate")

	// 新宿主实例沿用磁盘数据，分支恢复不依赖原宿主的内存。
	restarted := &backend.Core{Paths: core.Paths, FS: core.FS, Configs: core.Configs}
	restarted.InitServers()
	for i := 0; i < 2; i++ {
		if _, err := restarted.SetChatBranch(task.ID, userID, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("restart could not restore deleted branch", err)
		}
		if _, err := restarted.SetChatBranch(task.ID, userID, 1); err != nil {
			t.Fatal(err)
		}
		assertRecoveryContent(t, file, "alternate")
	}
}

func TestToolRecoveryEditedMessageStartsFromOriginalFiles(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("same", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old-branch"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			assertRecoveryContent(t, file, "original")
			return []api.ToolCall{approvalCall("same", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "edited-branch"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "original prompt", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	if err := core.EditChatMessage(task.ID, stored.Tree.Root.Next[0].ID, "edited prompt", ""); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "edited-branch")
	for _, branch := range []int{0, 1, 0} {
		if _, err := core.SetChatBranch(task.ID, "", branch); err != nil {
			t.Fatal(err)
		}
		want := "old-branch"
		if branch == 1 {
			want = "edited-branch"
		}
		assertRecoveryContent(t, file, want)
	}
}

func TestToolRecoveryInvalidOrMissingCacheBlocksBranch(t *testing.T) {
	for _, invalid := range []string{"missing", "corrupt", "identity"} {
		t.Run(invalid, func(t *testing.T) {
			core, _ := approvalCore(t)
			task, project := recoveryChat(t, core)
			file := filepath.Join(project, "note.txt")
			writeFileToolFixture(t, file, "original")
			approvalModel(t, core,
				func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("original", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old"})}
				}, nil,
				func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("alternate", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
				}, nil,
			)
			if err := core.SendChatMessage(task.ID, "original", ""); err != nil {
				t.Fatal(err)
			}
			stored, _ := recoveryOperations(t, core, task.ID)
			path, _ := stored.Tree.CurrentPath()
			if err := core.RegenerateChatMessage(task.ID, path[1].ID, ""); err != nil {
				t.Fatal(err)
			}
			stored, operations := recoveryOperations(t, core, task.ID)
			cache := recoveryCachePath(core, operations[0])
			saved, _ := os.ReadFile(cache)
			switch invalid {
			case "missing":
				if err := os.Remove(cache); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				writeFileToolFixture(t, cache, "{")
			case "identity":
				var record map[string]any
				_ = json.Unmarshal(saved, &record)
				record["callId"] = "another-call"
				data, _ := json.Marshal(record)
				writeFileToolFixture(t, cache, string(data))
			}
			if _, err := core.SetChatBranch(task.ID, path[0].ID, 0); err == nil {
				t.Fatal("missing or damaged recovery must block branch change")
			}
			assertRecoveryContent(t, file, "new")
			loaded, _ := recoveryOperations(t, core, task.ID)
			if loaded.Tree.FindNode(path[0].ID).Choice != stored.Tree.FindNode(path[0].ID).Choice {
				t.Fatal("failed transition persisted a new branch choice")
			}
			if err := os.WriteFile(cache, saved, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatBranch(task.ID, path[0].ID, 0); err != nil {
				t.Fatal(err)
			}
			assertRecoveryContent(t, file, "old")
		})
	}
}

type failChatCommitFS struct {
	platform.FilePlatform
	name string
	once bool
}

func (f *failChatCommitFS) MoveFile(source, target string) error {
	if filepath.Base(target) == f.name && !f.once {
		f.once = true
		return errors.New("injected chat commit failure")
	}
	return f.FilePlatform.MoveFile(source, target)
}

func TestToolRecoveryFailedBranchSaveKeepsFilesAndChoice(t *testing.T) {
	for _, saveFile := range []string{"session", "index"} {
		t.Run(saveFile, func(t *testing.T) {
			core, _ := approvalCore(t)
			task, project := recoveryChat(t, core)
			file := filepath.Join(project, "note.txt")
			writeFileToolFixture(t, file, "original")
			approvalModel(t, core,
				func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("old", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old"})}
				}, nil,
				func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("new", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
				}, nil,
			)
			if err := core.SendChatMessage(task.ID, "prompt", ""); err != nil {
				t.Fatal(err)
			}
			stored, _ := recoveryOperations(t, core, task.ID)
			path, _ := stored.Tree.CurrentPath()
			if err := core.RegenerateChatMessage(task.ID, path[1].ID, ""); err != nil {
				t.Fatal(err)
			}
			name := task.ID + ".json"
			if saveFile == "index" {
				name = "index.json"
			}
			core.FS = &failChatCommitFS{FilePlatform: core.FS, name: name}
			if _, err := core.SetChatBranch(task.ID, path[0].ID, 0); err == nil {
				t.Fatal("injected persistence failure did not fail branch switch")
			}
			assertRecoveryContent(t, file, "new")
			loaded, _ := recoveryOperations(t, core, task.ID)
			if loaded.Tree.FindNode(path[0].ID).Choice != 1 {
				t.Fatal("failed save left persisted Choice different from files")
			}
		})
	}
}

func TestToolRecoveryStrictApprovalUsesOriginalCall(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	if _, err := core.SetChatToolConfig(task.ID, `{"permission":1,"strictMode":true}`); err != nil {
		t.Fatal(err)
	}
	approvalEvents(t, func(payload map[string]string, view api.ToolApproval) {
		if view.CallID != "delete-original" || payload["callId"] != "request-delete" {
			t.Error("approval did not preserve original call identity")
		}
		if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], backend.ApprovalOnce); err != nil {
			t.Error(err)
		}
	})
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("delete-original", "DeleteFile", map[string]any{"path": []string{"note.txt"}})}
		},
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("request-delete", agenttool.RequestToolExecutionID, map[string]any{"call_id": "delete-original"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "delete", ""); err != nil {
		t.Fatal(err)
	}
	stored, operations := recoveryOperations(t, core, task.ID)
	if len(operations) != 1 || operations[0].CallID != "delete-original" || operations[0].ToolID != "DeleteFile" {
		t.Fatalf("recovery associated with approval request instead of file tool: %+v", operations)
	}
	path, _ := stored.Tree.CurrentPath()
	if err := core.RegenerateChatMessage(task.ID, path[1].ID, ""); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "original")
}

func TestToolRecoveryFailedEditSetupDoesNotChangeFiles(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "current"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "prompt", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	if err := os.Rename(project, project+"-moved"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(project+"-moved", project)
	if err := core.EditChatMessage(task.ID, path[0].ID, "new prompt", ""); err == nil {
		t.Fatal("missing workspace should fail setup")
	}
	assertRecoveryContent(t, filepath.Join(project+"-moved", "note.txt"), "current")
	loaded, _ := recoveryOperations(t, core, task.ID)
	if len(loaded.Tree.Root.Next) != 1 || loaded.Tree.Root.Choice != 0 {
		t.Fatal("failed setup persisted a new branch")
	}
}

type cancelRecoveryWriteFS struct {
	platform.FilePlatform
	cancel context.CancelFunc
	once   bool
}

func (f *cancelRecoveryWriteFS) WriteFileInRoot(root, name string, data []byte, mode fs.FileMode) error {
	err := f.FilePlatform.WriteFileInRoot(root, name, data, mode)
	if err == nil && name == "note.txt" && !f.once {
		f.once = true
		f.cancel()
	}
	return err
}

func TestToolRecoveryCancellationRollsBackAndPersistsNoOperation(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := testServer[*backend.ChatServer](t, core, backend.ChatServerName)
	if err := server.Start(parent); err != nil {
		t.Fatal(err)
	}
	core.FS = &cancelRecoveryWriteFS{FilePlatform: core.FS, cancel: cancel}
	approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
		return []api.ToolCall{approvalCall("canceled-write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "canceled"})}
	})
	if err := core.SendChatMessage(task.ID, "write then cancel", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	assertRecoveryContent(t, file, "original")
	_, operations := recoveryOperations(t, core, task.ID)
	if len(operations) != 0 {
		t.Fatalf("rolled-back cancellation left an active operation: %+v", operations)
	}
	files, _ := filepath.Glob(filepath.Join(core.Paths.Data, "InternalWorkspaces", "WorkspaceCache", task.WorkspaceID, "*.json"))
	if len(files) != 0 {
		t.Fatalf("successfully rolled-back operation retained unused copies: %v", files)
	}
}

func TestToolRecoveryCacheDoesNotEnterModelMessages(t *testing.T) {
	core, _ := approvalCore(t)
	task, _ := recoveryChat(t, core)
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
		},
		func(request api.ChatCompletionsRequest) []api.ToolCall {
			body, _ := json.Marshal(request.Messages)
			for _, forbidden := range []string{"toolOperations", "WorkspaceCache", "recovery-"} {
				if strings.Contains(string(body), forbidden) {
					t.Errorf("recovery metadata should be local: %s", body)
				}
			}
			return nil
		},
	)
	if err := core.SendChatMessage(task.ID, "create a file", ""); err != nil {
		t.Fatal(err)
	}
}

type failPartialRestoreFS struct {
	platform.FilePlatform
	once bool
}

func (f *failPartialRestoreFS) RestoreFileInRoot(root, name string, data []byte, mode fs.FileMode, modified time.Time) error {
	if !f.once {
		f.once = true
		if err := f.FilePlatform.RestoreFileInRoot(root, name, []byte("partial-restore"), mode, modified); err != nil {
			return err
		}
		return errors.New("injected partial restore failure")
	}
	return f.FilePlatform.RestoreFileInRoot(root, name, data, mode, modified)
}

func TestToolRecoveryPartialTransitionRollsBackFailedStep(t *testing.T) {
	for _, applyNew := range []bool{false, true} {
		t.Run(fmt.Sprint(applyNew), func(t *testing.T) {
			core, _ := approvalCore(t)
			task, project := recoveryChat(t, core)
			file := filepath.Join(project, "note.txt")
			writeFileToolFixture(t, file, "original")
			approvalModel(t, core,
				func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("old", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old"})}
				}, nil,
				func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("new", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
				}, nil,
			)
			if err := core.SendChatMessage(task.ID, "prompt", ""); err != nil {
				t.Fatal(err)
			}
			stored, _ := recoveryOperations(t, core, task.ID)
			path, _ := stored.Tree.CurrentPath()
			if err := core.RegenerateChatMessage(task.ID, path[1].ID, ""); err != nil {
				t.Fatal(err)
			}
			if applyNew {
				// 空分支离开时无操作，错误发生在进入目标分支的重放过程。
				if err := core.RegenerateChatMessage(task.ID, path[1].ID, ""); err != nil {
					t.Fatal(err)
				}
			}
			core.FS = &failPartialRestoreFS{FilePlatform: core.FS}
			if _, err := core.SetChatBranch(task.ID, path[0].ID, 0); err == nil {
				t.Fatal("partial restore must fail the branch transition")
			}
			want, choice := "new", 1
			if applyNew {
				want, choice = "original", 2
			}
			assertRecoveryContent(t, file, want)
			loaded, _ := recoveryOperations(t, core, task.ID)
			if loaded.Tree.FindNode(path[0].ID).Choice != choice {
				t.Fatal("partial transition changed persisted Choice")
			}
		})
	}
}

func TestToolRecoveryFailedRegenerationSaveRestoresExistingBranch(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "current"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "prompt", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	core.FS = &failChatCommitFS{FilePlatform: core.FS, name: "index.json"}
	if err := core.RegenerateChatMessage(task.ID, path[1].ID, ""); err == nil {
		t.Fatal("regeneration should fail when branch save cannot commit")
	}
	assertRecoveryContent(t, file, "current")
	loaded, _ := recoveryOperations(t, core, task.ID)
	user := loaded.Tree.FindNode(path[0].ID)
	if user.Choice != 0 || len(user.Next) != 1 {
		t.Fatal("failed regeneration persisted a partially selected new branch")
	}
}
