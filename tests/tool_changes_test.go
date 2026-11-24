package Tests

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	backend "github.com/caliniya/caf/core/Backend"
)

func TestChatToolChangesUndoRedoOrderingAndRestart(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original\n")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "first\n"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{
				approvalCall("create", "WriteFile", map[string]any{"path": []string{"created.txt"}, "content": "created\n"}),
				approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "second\n"}),
				approvalCall("delete", "DeleteFile", map[string]any{"path": []string{"note.txt"}}),
			}
		}, nil,
	)
	for _, message := range []string{"first round", "second round"} {
		if err := core.SendChatMessage(task.ID, message, ""); err != nil {
			t.Fatal(err)
		}
	}
	stored, operations := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	firstReply, secondReply := path[1].ID, path[3].ID
	if len(operations) != 4 || len(operations[0].Changes) != 1 || operations[0].Changes[0].AddedLines != 1 || operations[0].Changes[0].RemovedLines != 1 ||
		operations[1].Changes[0].Action != "create" || operations[3].Changes[0].Action != "delete" {
		t.Fatal("actual change summaries were not persisted", operations)
	}
	unchangedMessages := append([]api.ChatMessage(nil), path[3].Child...)
	if _, err := core.SetChatToolChangesUndone(task.ID, firstReply, true); err == nil || !strings.Contains(err.Error(), "后续") {
		t.Fatal("out-of-order undo should be rejected", err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, path[0].ID, true); err == nil {
		t.Fatal("user message should not accept a tool undo")
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, secondReply, true); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "first\n")
	if _, err := os.Stat(filepath.Join(project, "created.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("undo did not remove the newly created file", err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, secondReply, true); err != nil {
		t.Fatal("repeated undo should succeed", err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, firstReply, true); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "original\n")
	if _, err := core.SetChatToolChangesUndone(task.ID, secondReply, false); err == nil || !strings.Contains(err.Error(), "此前") {
		t.Fatal("out-of-order redo should be rejected", err)
	}
	// 重启后仍能读取撤销状态，恢复过程中不请求模型。
	restarted := &backend.Core{Paths: core.Paths, FS: core.FS, Configs: core.Configs}
	restarted.InitServers()
	for _, reply := range []string{firstReply, secondReply} {
		if _, err := restarted.SetChatToolChangesUndone(task.ID, reply, false); err != nil {
			t.Fatal(err)
		}
	}
	assertRecoveryContent(t, filepath.Join(project, "created.txt"), "created\n")
	if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("redo did not apply the final deletion", err)
	}
	if _, err := restarted.SetChatToolChangesUndone(task.ID, secondReply, false); err != nil {
		t.Fatal("repeated redo should succeed", err)
	}
	stored, current := recoveryOperations(t, restarted, task.ID)
	if !reflect.DeepEqual(current, operations) || !reflect.DeepEqual(stored.Tree.FindNode(secondReply).Child, unchangedMessages) {
		t.Fatal("undo/redo changed summaries or conversation messages")
	}
}

func TestChatToolChangesRespectUndoneBranchState(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "baseline")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			assertRecoveryContent(t, file, "baseline")
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "write", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	userID, replyID := path[0].ID, path[1].ID
	if err := core.RegenerateChatMessage(task.ID, replyID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, replyID, true); err == nil {
		t.Fatal("inactive branch should not accept undo")
	}
	if _, err := core.SetChatBranch(task.ID, userID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, replyID, true); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "baseline")
	if _, err := core.SetChatBranch(task.ID, userID, 1); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "new")
	if _, err := core.SetChatBranch(task.ID, userID, 0); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "baseline")
	if _, err := core.SetChatToolChangesUndone(task.ID, replyID, false); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "old")
}

func TestChatToolChangesNewWriteDiscardsRedoAndInformsModel(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "baseline")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("old-write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old"})}
		}, nil,
		func(request api.ChatCompletionsRequest) []api.ToolCall {
			notice := false
			for _, message := range request.Messages {
				if message.Role == "system" && strings.Contains(message.Content.(string), "用户已撤销") && strings.Contains(message.Content.(string), "old-write") {
					notice = true
				}
			}
			if !notice {
				t.Error("model was not informed of user undo")
			}
			return []api.ToolCall{approvalCall("new-write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "old write", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	oldReply := path[1].ID
	if _, err := core.SetChatToolChangesUndone(task.ID, oldReply, true); err != nil {
		t.Fatal(err)
	}
	if err := core.SendChatMessage(task.ID, "new write", ""); err != nil {
		t.Fatal(err)
	}
	stored, operations := recoveryOperations(t, core, task.ID)
	if operations[0].State != agenttool.ToolOperationDiscarded || operations[1].State != "" {
		t.Fatal("redo was not discarded after a successful new write", operations)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, oldReply, false); err == nil {
		t.Fatal("discarded redo should not overwrite a new file")
	}
	assertRecoveryContent(t, file, "new")
	path, _ = stored.Tree.CurrentPath()
	if _, err := core.SetChatToolChangesUndone(task.ID, path[3].ID, true); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "baseline")
	if _, err := core.SetChatToolChangesUndone(task.ID, oldReply, false); err == nil {
		t.Fatal("discarded redo unexpectedly became available again")
	}
}

func TestChatToolChangesFailedPersistenceRestoresFilesAndState(t *testing.T) {
	for _, name := range []string{"session", "index"} {
		for _, undone := range []bool{true, false} {
			t.Run(name+"/"+map[bool]string{true: "undo", false: "redo"}[undone], func(t *testing.T) {
				core, _ := approvalCore(t)
				task, project := recoveryChat(t, core)
				file := filepath.Join(project, "note.txt")
				writeFileToolFixture(t, file, "original")
				approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
					return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "changed"})}
				}, nil)
				if err := core.SendChatMessage(task.ID, "write", ""); err != nil {
					t.Fatal(err)
				}
				stored, _ := recoveryOperations(t, core, task.ID)
				path, _ := stored.Tree.CurrentPath()
				reply := path[1].ID
				want, wantState := "changed", ""
				if !undone {
					if _, err := core.SetChatToolChangesUndone(task.ID, reply, true); err != nil {
						t.Fatal(err)
					}
					want, wantState = "original", agenttool.ToolOperationUndone
				}
				fileName := task.ID + ".json"
				if name == "index" {
					fileName = "index.json"
				}
				core.FS = &failChatCommitFS{FilePlatform: core.FS, name: fileName}
				if _, err := core.SetChatToolChangesUndone(task.ID, reply, undone); err == nil {
					t.Fatal("injected commit failure should fail undo/redo")
				}
				assertRecoveryContent(t, file, want)
				_, operations := recoveryOperations(t, core, task.ID)
				if operations[0].State != wantState {
					t.Fatal("failed commit changed persisted undo state", operations)
				}
			})
		}
	}
}

func TestChatToolChangesMissingCacheKeepsState(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
		return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "changed"})}
	}, nil)
	if err := core.SendChatMessage(task.ID, "write", ""); err != nil {
		t.Fatal(err)
	}
	stored, operations := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	if err := os.Remove(recoveryCachePath(core, operations[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, path[1].ID, true); err == nil {
		t.Fatal("missing cache should fail undo")
	}
	assertRecoveryContent(t, file, "changed")
	_, operations = recoveryOperations(t, core, task.ID)
	if operations[0].State != "" {
		t.Fatal("failed undo persisted a changed state")
	}
}

func TestChatToolChangesMetadataStaysOutOfModelHistory(t *testing.T) {
	core, _ := approvalCore(t)
	task, _ := recoveryChat(t, core)
	approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
		return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "new"})}
	}, func(request api.ChatCompletionsRequest) []api.ToolCall {
		data, _ := json.Marshal(request.Messages)
		for _, forbidden := range []string{"toolOperations", "addedLines", "removedLines", "beforeBytes", "afterBytes", "recovery-"} {
			if strings.Contains(string(data), forbidden) {
				t.Error("presentation metadata entered the model history", string(data))
			}
		}
		return nil
	})
	if err := core.SendChatMessage(task.ID, "write", ""); err != nil {
		t.Fatal(err)
	}
}

func TestChatToolChangesPartialRestoreFailureRollsBackWholeRound(t *testing.T) {
	for _, undone := range []bool{true, false} {
		t.Run(map[bool]string{true: "undo", false: "redo"}[undone], func(t *testing.T) {
			core, _ := approvalCore(t)
			task, project := recoveryChat(t, core)
			file := filepath.Join(project, "note.txt")
			writeFileToolFixture(t, file, "original")
			approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
				return []api.ToolCall{
					approvalCall("first", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "first"}),
					approvalCall("second", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "second"}),
				}
			}, nil)
			if err := core.SendChatMessage(task.ID, "write twice", ""); err != nil {
				t.Fatal(err)
			}
			stored, _ := recoveryOperations(t, core, task.ID)
			path, _ := stored.Tree.CurrentPath()
			reply := path[1].ID
			want, state := "second", ""
			if !undone {
				if _, err := core.SetChatToolChangesUndone(task.ID, reply, true); err != nil {
					t.Fatal(err)
				}
				want, state = "original", agenttool.ToolOperationUndone
			}
			core.FS = &failPartialRestoreFS{FilePlatform: core.FS}
			if _, err := core.SetChatToolChangesUndone(task.ID, reply, undone); err == nil {
				t.Fatal("partial file restore should fail undo/redo")
			}
			assertRecoveryContent(t, file, want)
			_, operations := recoveryOperations(t, core, task.ID)
			for _, operation := range operations {
				if operation.State != state {
					t.Fatal("partial failure changed operation state", operations)
				}
			}
		})
	}
}

func TestChatToolChangesFailedNewWritePreservesRedo(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("old-write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "old"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("bad-write", "WriteFile", map[string]any{"path": []string{"missing", "note.txt"}, "content": "bad"})}
		}, nil,
	)
	if err := core.SendChatMessage(task.ID, "old write", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	reply := path[1].ID
	if _, err := core.SetChatToolChangesUndone(task.ID, reply, true); err != nil {
		t.Fatal(err)
	}
	if err := core.SendChatMessage(task.ID, "failed new write", ""); err != nil {
		t.Fatal(err)
	}
	_, operations := recoveryOperations(t, core, task.ID)
	if len(operations) != 1 || operations[0].State != agenttool.ToolOperationUndone {
		t.Fatal("failed new write should preserve earlier redo", operations)
	}
	assertRecoveryContent(t, file, "original")
	if _, err := core.SetChatToolChangesUndone(task.ID, reply, false); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "old")
}

func TestChatToolChangesUndoneAncestorAcrossExistingDescendantBranches(t *testing.T) {
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "baseline")
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("ancestor", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "ancestor"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("first-child", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "first-child"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("second-child", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "second-child"})}
		}, nil,
	)
	for _, message := range []string{"ancestor", "child"} {
		if err := core.SendChatMessage(task.ID, message, ""); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	ancestor, childUser, firstChild := path[1].ID, path[2].ID, path[3].ID
	if err := core.RegenerateChatMessage(task.ID, firstChild, ""); err != nil {
		t.Fatal(err)
	}
	stored, _ = recoveryOperations(t, core, task.ID)
	path, _ = stored.Tree.CurrentPath()
	secondChild := path[3].ID
	if _, err := core.SetChatToolChangesUndone(task.ID, secondChild, true); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, ancestor, true); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "baseline")
	if _, err := core.SetChatBranch(task.ID, childUser, 0); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "first-child")
	// 这个副本的 Before 含有祖先改动，撤销时仍需回到已撤销祖先之前的底稿。
	if _, err := core.SetChatToolChangesUndone(task.ID, firstChild, true); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "baseline")
	if _, err := core.SetChatToolChangesUndone(task.ID, ancestor, false); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "ancestor")
	if _, err := core.SetChatToolChangesUndone(task.ID, firstChild, false); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, file, "first-child")
}

func TestChatToolChangesRejectsUndoDuringGeneration(t *testing.T) {
	core, _ := approvalCore(t)
	task, _ := recoveryChat(t, core)
	started, release := make(chan struct{}), make(chan struct{})
	approvalModel(t, core,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": "changed"})}
		}, nil,
		func(api.ChatCompletionsRequest) []api.ToolCall {
			close(started)
			<-release
			return nil
		},
	)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	if err := core.SendChatMessage(task.ID, "write", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := recoveryOperations(t, core, task.ID)
	path, _ := stored.Tree.CurrentPath()
	done := make(chan error, 1)
	go func() { done <- core.SendChatMessage(task.ID, "continue", "") }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("mock model did not start")
	}
	if _, err := core.SetChatToolChangesUndone(task.ID, path[1].ID, true); err == nil || !strings.Contains(err.Error(), "生成") {
		t.Fatal("undo must be rejected during generation", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_, operations := recoveryOperations(t, core, task.ID)
	if operations[0].State != "" {
		t.Fatal("rejected undo changed the operation state")
	}
}
