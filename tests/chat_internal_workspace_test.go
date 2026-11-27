package Tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	identity "github.com/caliniya/caf/utils/id"
	texts "github.com/caliniya/caf/utils/text"
)

func newWorkspaceChat(t *testing.T, core *backend.Core) *chat.ChatTask {
	t.Helper()
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected %s to be absent: %v", path, err)
	}
}

func TestChatInternalWorkspaceIsolationAndRecovery(t *testing.T) {
	c, _ := approvalCore(t)
	root := filepath.Join(t.TempDir(), "custom-internal")
	c.Paths.InternalWorkspaces = root
	a, b := newWorkspaceChat(t, c), newWorkspaceChat(t, c)
	aDir, bDir := filepath.Join(root, a.ID), filepath.Join(root, b.ID)
	for _, dir := range []string{aDir, bDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("missing allocated directory %s: %v", dir, err)
		}
	}
	project, err := c.CreateWorkspace("外部项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.ListWorkspaces()
	if err != nil || len(list) != 1 || list[0].ID != project.ID || a.WorkspaceID != "" || b.WorkspaceID != "" {
		t.Fatalf("internal workspaces leaked into external workspace selection: %+v / %v", list, err)
	}
	write := func(dir, content string) func(api.ChatCompletionsRequest) []api.ToolCall {
		return func(request api.ChatCompletionsRequest) []api.ToolCall {
			if !slices.ContainsFunc(request.Messages, func(msg api.CCChatMessage) bool {
				return msg.Role == "system" && msg.Content == fmt.Sprintf(texts.ChatWorkspace, dir)
			}) {
				t.Errorf("model did not receive the active workspace %s", dir)
			}
			return []api.ToolCall{approvalCall("write", "WriteFile", map[string]any{"path": []string{"note.txt"}, "content": content})}
		}
	}
	approvalModel(t, c, write(aDir, "A"), nil, write(bDir, "B"), nil, write(project.Path, "project"), nil, write(aDir, "A2"), nil)
	approvalEvents(t, func(payload map[string]string, view api.ToolApproval) {
		t.Errorf("automatic edit inside the active workspace unexpectedly required approval: %+v", view)
		if _, err := c.SubmitToolApproval(payload["chatId"], payload["requestId"], backend.ApprovalReject); err != nil {
			t.Error(err)
		}
	})
	send := func(c *backend.Core, chatID string) {
		t.Helper()
		if err := c.SendChatMessage(chatID, "写 note.txt", ""); err != nil {
			t.Fatal(err)
		}
	}
	send(c, a.ID)
	send(c, b.ID)
	aFile, bFile := filepath.Join(aDir, "note.txt"), filepath.Join(bDir, "note.txt")
	assertRecoveryContent(t, aFile, "A")
	assertRecoveryContent(t, bFile, "B")
	stored, aOps := recoveryOperations(t, c, a.ID)
	_, bOps := recoveryOperations(t, c, b.ID)
	if len(aOps) != 1 || len(bOps) != 1 || !identity.Valid(aOps[0].WorkspaceID, identity.Workspace) || aOps[0].WorkspaceID == bOps[0].WorkspaceID {
		t.Fatalf("expected separate recovery scopes: %+v / %+v", aOps, bOps)
	}
	aCache, bCache := recoveryCachePath(c, aOps[0]), recoveryCachePath(c, bOps[0])
	for _, path := range []string{aCache, bCache} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	assertPathAbsent(t, filepath.Join(root, "WorkspaceCache", "unscoped"))
	restarted := &backend.Core{Paths: c.Paths, FS: c.FS, Configs: c.Configs}
	restarted.InitServers()
	if _, err := restarted.SetChatWorkspace(a.ID, project.ID); err != nil {
		t.Fatal(err)
	}
	// 即便当前指向外部项目，撤销和重做仍作用于原调用所在的内部目录。
	path, _ := stored.Tree.CurrentPath()
	if _, err := restarted.SetChatToolChangesUndone(a.ID, path[1].ID, true); err != nil {
		t.Fatal(err)
	}
	assertPathAbsent(t, aFile)
	assertRecoveryContent(t, bFile, "B")
	if _, err := restarted.SetChatToolChangesUndone(a.ID, path[1].ID, false); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, aFile, "A")
	send(restarted, a.ID)
	if _, err := restarted.SetChatWorkspace(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	send(restarted, a.ID)
	assertRecoveryContent(t, aFile, "A2")
	assertRecoveryContent(t, filepath.Join(project.Path, "note.txt"), "project")
	if err := restarted.DeleteChat(a.ID); err != nil {
		t.Fatal(err)
	}
	assertPathAbsent(t, aDir)
	assertPathAbsent(t, filepath.Dir(aCache))
	assertRecoveryContent(t, bFile, "B")
	if _, err := os.Stat(bCache); err != nil {
		t.Fatal(err)
	}
	assertRecoveryContent(t, filepath.Join(project.Path, "note.txt"), "project")
}

func TestChatInternalWorkspaceApprovalScope(t *testing.T) {
	c, _ := approvalCore(t)
	a, b := newWorkspaceChat(t, c), newWorkspaceChat(t, c)
	project, err := c.CreateWorkspace("外部项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetChatToolConfig(a.ID, `{"permission":1}`); err != nil {
		t.Fatal(err)
	}
	write := func(id, path string) api.ToolCall {
		return approvalCall(id, "WriteFile", map[string]any{"path": []string{path}, "content": id})
	}
	approvalModel(t, c, func(api.ChatCompletionsRequest) []api.ToolCall {
		return []api.ToolCall{write("first", "first.txt"), write("second", "second.txt"), write("sibling", filepath.Join("..", b.ID, "second.txt"))}
	}, nil, func(api.ChatCompletionsRequest) []api.ToolCall {
		return []api.ToolCall{write("external", "external.txt")}
	})
	var asked []string
	approvalEvents(t, func(payload map[string]string, view api.ToolApproval) {
		asked = append(asked, view.CallID)
		decision := backend.ApprovalReject
		switch view.CallID {
		case "first":
			if !view.WorkspaceAllowed || !strings.Contains(view.WorkspaceName, "内部工作区") || view.HigherPermission != 2 {
				t.Errorf("wrong internal workspace approval: %+v", view)
			}
			decision = backend.ApprovalWorkspace
		case "sibling":
			if view.WorkspaceAllowed {
				t.Error("one chat could grant access to another chat's directory")
			}
		case "external":
			if view.WorkspaceID != project.ID {
				t.Error("approval used the internal ID after switching to an external workspace")
			}
		}
		if _, err := c.SubmitToolApproval(a.ID, payload["requestId"], decision); err != nil {
			t.Error(err)
		}
	})
	if err := c.SendChatMessage(a.ID, "写入文件", ""); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(c.Paths.Data, "InternalWorkspaces")
	assertRecoveryContent(t, filepath.Join(root, a.ID, "second.txt"), "second")
	assertPathAbsent(t, filepath.Join(root, b.ID, "second.txt"))
	loaded, err := c.SetChatWorkspace(a.ID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(loaded.ToolConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetChatToolConfig(a.ID, string(config)); err != nil {
		t.Fatal("cannot keep own internal grant after switching workspaces", err)
	}
	if _, err := c.SetChatToolConfig(b.ID, `{"permission":1}`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetChatToolConfig(b.ID, string(config)); err == nil {
		t.Fatal("another chat accepted the internal grant")
	}
	if err := c.SendChatMessage(a.ID, "写外部项目", ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(asked, []string{"first", "sibling", "external"}) {
		t.Fatalf("unexpected approval requests: %v", asked)
	}
	assertPathAbsent(t, filepath.Join(project.Path, "external.txt"))
}

func TestChatInternalWorkspaceCompactionReads(t *testing.T) {
	c, _ := approvalCore(t)
	a := newWorkspaceChat(t, c)
	dir := filepath.Join(c.Paths.Data, "InternalWorkspaces", a.ID)
	seedCompactionChat(t, c, a, 5, 4096)
	writeFileToolFixture(t, filepath.Join(dir, "note.txt"), "from this chat")
	compactionHTTPModel(t, c, func(w http.ResponseWriter, _ *http.Request, request api.ChatCompletionsRequest) {
		switch request.Messages[0].Content {
		case texts.CompactionSummarize:
			var source struct{ State []string }
			input, _ := request.Messages[1].Content.(string)
			if json.Unmarshal([]byte(input), &source) != nil || !slices.Contains(source.State, fmt.Sprintf(texts.ChatWorkspace, dir)) {
				t.Error("summary did not receive the internal workspace path")
			}
			qqSSE(w, compactionCandidateText("继续读取 note.txt", compactionReadCall()))
		case texts.CompactionReview:
			qqSSE(w, `{"passed":true,"issues":[]}`)
		default:
			t.Error("unexpected compaction request")
			qqSSE(w, "unexpected")
		}
	})
	compacted, err := c.CompactChat(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, data, err := compacted.Tree.CurrentCompaction()
	if err != nil || data == nil || len(data.Results) != 2 || data.Results[1].Content != "from this chat" {
		t.Fatalf("compaction did not restore the relative file: %+v / %v", data, err)
	}
}

func TestChatInternalWorkspaceWhenRemovingProject(t *testing.T) {
	for _, deleteChats := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleteChats=%t", deleteChats), func(t *testing.T) {
			c := useChatTaskCore(t)
			a := newWorkspaceChat(t, c)
			dir := filepath.Join(c.Paths.Data, "InternalWorkspaces", a.ID)
			project, err := c.CreateWorkspace("项目", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			writeFileToolFixture(t, filepath.Join(project.Path, "note.txt"), "project")
			if _, err := c.SetChatWorkspace(a.ID, project.ID); err != nil {
				t.Fatal(err)
			}
			if deleteChats {
				writeFileToolFixture(t, filepath.Join(dir, "note.txt"), "private")
				if err := c.DeleteWorkspaceAndChats(project.ID); err != nil {
					t.Fatal(err)
				}
				assertPathAbsent(t, dir)
			} else {
				// 老对话可能尚未分配内部目录。
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
				if err := c.DeleteWorkspace(project.ID); err != nil {
					t.Fatal(err)
				}
				if loaded, err := c.GetChat(a.ID); err != nil || loaded.WorkspaceID != "" {
					t.Fatalf("chat was not detached: %+v / %v", loaded, err)
				}
				if info, err := os.Stat(dir); err != nil || !info.IsDir() {
					t.Fatal("no internal workspace after detaching old chat", err)
				}
			}
			assertRecoveryContent(t, filepath.Join(project.Path, "note.txt"), "project")
		})
	}
}
