package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	platform "github.com/caliniya/caf/platform"
	texts "github.com/caliniya/caf/utils/text"
)

func persistCompactionChat(t *testing.T, core *backend.Core, task *chat.ChatTask) {
	t.Helper()
	body, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core.Paths.Data, "chats", task.ID+".json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedCompactionChat(t *testing.T, core *backend.Core, task *chat.ChatTask, rounds, tokens int) []*chatstruct.ChatNode {
	t.Helper()
	parent := ""
	nodes := make([]*chatstruct.ChatNode, 0, rounds*2)
	for index := range rounds {
		user := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: fmt.Sprintf("old-user-%d ", index) + strings.Repeat("history-detail ", 80), Tokens: tokens})
		reply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: fmt.Sprintf("old-answer-%d", index), Tokens: tokens})
		if err := task.Tree.AddNext(parent, user); err != nil {
			t.Fatal(err)
		}
		user.AddNext(reply)
		parent = reply.ID
		nodes = append(nodes, user, reply)
	}
	persistCompactionChat(t, core, task)
	return nodes
}

type compactionReadCounter struct {
	platform.FilePlatform
	target  string
	reads   atomic.Int32
	readErr error
}

func (f *compactionReadCounter) ReadFile(name string) ([]byte, error) {
	if filepath.Clean(name) == filepath.Clean(f.target) {
		f.reads.Add(1)
		if f.readErr != nil {
			return nil, f.readErr
		}
	}
	return f.FilePlatform.ReadFile(name)
}

func useCompactionChat(t *testing.T) (*backend.Core, *chat.ChatTask, string, *compactionReadCounter) {
	t.Helper()
	core, _ := approvalCore(t)
	task, project := recoveryChat(t, core)
	task, err := core.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "note.txt")
	writeFileToolFixture(t, file, "original-file")
	counter := &compactionReadCounter{FilePlatform: core.FS, target: file}
	core.FS = counter
	seedCompactionChat(t, core, task, 5, 4096)
	return core, task, file, counter
}

func compactionHTTPModel(t *testing.T, core *backend.Core, respond func(http.ResponseWriter, *http.Request, api.ChatCompletionsRequest)) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request api.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		respond(w, r, request)
	}))
	t.Cleanup(server.Close)
	core.Configs.EnablesConfig.ServerAddress = server.URL
}

func compactionRequestText(request api.ChatCompletionsRequest) string {
	var text strings.Builder
	for _, message := range request.Messages {
		fmt.Fprint(&text, message.Content)
	}
	return text.String()
}

func compactionCandidateText(summary string, restore ...chatstruct.CompactionRestoreCall) string {
	if restore == nil {
		restore = []chatstruct.CompactionRestoreCall{}
	}
	body, _ := json.Marshal(map[string]any{"summary": summary, "restore": restore})
	return string(body)
}

func compactionReadCall() chatstruct.CompactionRestoreCall {
	return chatstruct.CompactionRestoreCall{Tool: "ReadFile", Arguments: json.RawMessage(`{"path":["note.txt"]}`), Purpose: "恢复正在编辑的文件"}
}

func TestCompactionPlanUsesNewTokensAndKeepsToolGroups(t *testing.T) {
	tree := chatstruct.CreateTree("tokens")
	parent := ""
	users := make([]string, 4)
	for index := range 4 {
		user := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: fmt.Sprint(index), Tokens: 4096})
		reply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "answer", Tokens: 4096})
		reply.Usage = &api.TokenUsage{PromptTokens: 300000, CompletionTokens: 4096, TotalTokens: 304096}
		if err := tree.AddNext(parent, user); err != nil {
			t.Fatal(err)
		}
		user.AddNext(reply)
		users[index], parent = user.ID, reply.ID
	}
	plan, err := tree.NewCompactionPlan(chatstruct.CompactionKeepTokens)
	if err != nil || plan.Start.NodeID != users[2] || len(plan.Recent) != 4 {
		t.Fatalf("repeated prompt tokens changed the recent boundary: %+v / %v", plan, err)
	}
	body, _ := json.Marshal(plan)
	if strings.Contains(string(body), "tokens") || strings.Contains(string(body), "usage") {
		t.Fatal("accounting metadata leaked into the summary input")
	}
	user := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: "read", Tokens: 10})
	reply := chatstruct.CreateChatNode(
		api.ChatMessage{Role: "assistant", ToolCalls: []api.ToolCall{{ID: "read", Name: "ReadFile", Arguments: "{}"}}, Tokens: 10},
		api.ChatMessage{Role: "tool", ToolCallID: "read", Content: "large result", Tokens: 2000},
		api.ChatMessage{Role: "assistant", Content: "finished", Tokens: 10},
	)
	tree.AddNext(parent, user)
	user.AddNext(reply)
	plan, err = tree.NewCompactionPlan(2020)
	if err != nil || plan.Start.NodeID != user.ID {
		t.Fatalf("tool call and result were split: %+v / %v", plan, err)
	}
	if len(plan.Recent) != 4 || plan.Recent[1].ToolCalls[0].ID != plan.Recent[2].ToolCallID {
		t.Fatal("recent tool group is incomplete")
	}
}

func TestCompactionContextFollowsSelectedBranchAndKeepsOriginalHistory(t *testing.T) {
	tree := chatstruct.CreateTree("branches")
	first := tree.Start(api.ChatMessage{Role: "user", Content: "old question"})
	firstReply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "old answer"})
	first.AddNext(firstReply)
	second := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: "second question"})
	firstReply.AddNext(second)
	secondReply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "second answer"})
	second.AddNext(secondReply)
	secondReply.Compaction = &chatstruct.CompactionData{Start: chatstruct.MessagePosition{NodeID: second.ID}, Summary: "summary-A"}
	third := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: "third question"})
	secondReply.AddNext(third)
	thirdReply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "third answer"})
	third.AddNext(thirdReply)
	thirdReply.Compaction = &chatstruct.CompactionData{Start: chatstruct.MessagePosition{NodeID: third.ID}, Summary: "summary-B"}
	request, err := tree.ToChatRequest(api.Model{ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Messages) != 4 || !strings.Contains(request.Messages[1].Content, "summary-B") || request.Messages[2].Content != "third question" {
		t.Fatalf("newest compaction did not replace only its covered prefix: %+v", request)
	}
	raw, err := first.GetMessages()
	if err != nil || len(raw) != 6 || raw[0].Content != "old question" {
		t.Fatal("compaction changed the original tree", raw, err)
	}
	alternate := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: "alternate question"})
	secondReply.AddNext(alternate)
	request, _ = tree.ToChatRequest(api.Model{ID: "test"})
	if len(request.Messages) != 5 || !strings.Contains(request.Messages[1].Content, "summary-A") || request.Messages[4].Content != "alternate question" {
		t.Fatal("shared ancestor summary was not reused", request)
	}
	second.AddNext(chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "alternate answer"}))
	request, _ = tree.ToChatRequest(api.Model{ID: "test"})
	for _, message := range request.Messages {
		if strings.Contains(message.Content, "summary-") {
			t.Fatal("summary leaked from an unselected branch")
		}
	}
}

func TestCompactChatPersistsReviewedSummaryAndRefreshesReadResults(t *testing.T) {
	core, task, file, counter := useCompactionChat(t)
	var requests atomic.Int32
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		switch index := requests.Add(1); index {
		case 1:
			if len(request.Tools) != 0 || len(request.Messages) != 2 || request.Messages[0].Content != texts.CompactionSummarize {
				t.Error("summary request should be independent and must not execute tools")
			}
			var source struct {
				History []chatstruct.CompactionTurn `json:"history"`
				Recent  []api.ChatMessage           `json:"recent"`
			}
			input, _ := request.Messages[1].Content.(string)
			if json.Unmarshal([]byte(input), &source) != nil || len(source.History) != 6 || len(source.Recent) != 4 {
				t.Error("summary input did not preserve the fixed history and recent region")
			}
			if counter.reads.Load() != 0 {
				t.Error("restore tools executed before review")
			}
			qqSSE(w, "```json\n"+compactionCandidateText("用户要求继续完成文件工作。早期讨论已完成，下一步读取 note.txt。", compactionReadCall())+"\n```")
		case 2:
			if request.Messages[0].Content != texts.CompactionReview || counter.reads.Load() != 0 {
				t.Error("review was not independent or restore ran before approval")
			}
			current, err := core.GetChat(task.ID)
			if err != nil {
				t.Error(err)
			} else if _, data, _ := current.Tree.CurrentCompaction(); data != nil {
				t.Error("unreviewed candidate was saved")
			}
			if err := os.WriteFile(file, []byte("after-review"), 0o600); err != nil {
				t.Error(err)
			}
			qqSSE(w, `{"passed":true,"issues":[]}`)
		case 3:
			text := compactionRequestText(request)
			for _, required := range []string{"用户要求继续完成文件工作", "old-user-3", "continue task", "fresh-read", texts.PersonaPrefix} {
				if !strings.Contains(text, required) {
					t.Errorf("continued request omitted %q", required)
				}
			}
			if strings.Contains(text, "old-user-0") || strings.Contains(text, "after-review") {
				t.Error("continued request included compacted raw history or stale read results")
			}
			qqSSEUsage(w, "continued", &api.TokenUsage{PromptTokens: 1000, CompletionTokens: 27, TotalTokens: 1027}, 1)
		default:
			t.Errorf("unexpected request %d", index)
			qqSSE(w, "unexpected")
		}
	})
	compressed, err := core.CompactChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	marker, data, err := compressed.Tree.CurrentCompaction()
	if err != nil || marker == nil || data == nil || len(data.Results) != 2 || data.Results[1].Content != "after-review" {
		t.Fatalf("reviewed compaction and actual read results were not persisted: %+v / %v", data, err)
	}
	raw, _ := compressed.Tree.Root.Next[0].GetMessages()
	if len(raw) != 10 || len(marker.ToolOperations) != 0 || counter.reads.Load() != 1 {
		t.Fatal("compaction altered original messages, created changes, or repeated reads")
	}
	if err := os.WriteFile(file, []byte("fresh-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := core.SendChatMessage(task.ID, "continue task", "high"); err != nil {
		t.Fatal(err)
	}
	loaded, err := core.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := loaded.Tree.CurrentPath()
	if path[len(path)-1].Child[0].Tokens != 31 || counter.reads.Load() != 2 {
		t.Fatal("new token accounting or restore refresh failed")
	}
	encoded, _ := (&api.ChatCompletionAPI{}).EncodeRequest(api.ChatRequest{Model: "test", Messages: path[len(path)-1].Child})
	if strings.Contains(encoded, `"tokens"`) {
		t.Fatal("internal token accounting was sent to the provider")
	}
}

func TestCompactChatReviewRetryUsesIdenticalSnapshotAndRequest(t *testing.T) {
	core, task, _, counter := useCompactionChat(t)
	var attempts, reviews atomic.Int32
	var summaryRequests []api.ChatCompletionsRequest
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if request.Messages[0].Content == texts.CompactionSummarize {
			summaryRequests = append(summaryRequests, request)
			attempts.Add(1)
			qqSSE(w, compactionCandidateText("保持用户要求，继续文件工作。", compactionReadCall()))
			return
		}
		if counter.reads.Load() != 0 {
			t.Error("restore executed before the successful review")
		}
		if reviews.Add(1) == 1 {
			qqSSE(w, `{"passed":false,"issues":["测试：遗漏约束"]}`)
		} else {
			qqSSE(w, `{"passed":true,"issues":[]}`)
		}
	})
	if _, err := core.CompactChat(task.ID); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || reviews.Load() != 2 || !reflect.DeepEqual(summaryRequests[0], summaryRequests[1]) || counter.reads.Load() != 1 {
		t.Fatal("retry changed the input, attached review feedback, or executed restore early")
	}
}

func TestCompactChatFailedReviewAndUnsafeRestoreDoNotChangeHistory(t *testing.T) {
	for _, mode := range []string{"review", "delete", "write"} {
		t.Run(mode, func(t *testing.T) {
			core, task, file, counter := useCompactionChat(t)
			path := filepath.Join(core.Paths.Data, "chats", task.ID+".json")
			before, _ := os.ReadFile(path)
			var summaries, reviews atomic.Int32
			compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
				if request.Messages[0].Content == texts.CompactionReview {
					reviews.Add(1)
					qqSSE(w, `{"passed":false,"issues":["测试：不通过"]}`)
					return
				}
				summaries.Add(1)
				call := compactionReadCall()
				if mode == "delete" {
					call.Tool = "DeleteFile"
				} else if mode == "write" {
					call.Tool, call.Arguments = "WriteFile", json.RawMessage(`{"path":["note.txt"],"content":"bad"}`)
				}
				qqSSE(w, compactionCandidateText("摘要", call))
			})
			if _, err := core.CompactChat(task.ID); err == nil {
				t.Fatal("failed review or mutating restore was accepted")
			}
			after, _ := os.ReadFile(path)
			content, _ := os.ReadFile(file)
			if !reflect.DeepEqual(before, after) || string(content) != "original-file" || counter.reads.Load() != 0 || summaries.Load() != 3 {
				t.Fatal("failed compaction changed history or executed recovery tools")
			}
			if (mode == "review" && reviews.Load() != 3) || (mode != "review" && reviews.Load() != 0) {
				t.Fatal("unsafe restores reached the model review or review retry count was incorrect")
			}
		})
	}
}

func TestCompactChatCancellationDoesNotCommitCandidate(t *testing.T) {
	core, task, _, counter := useCompactionChat(t)
	reviewStarted := make(chan struct{})
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if request.Messages[0].Content == texts.CompactionSummarize {
			qqSSE(w, compactionCandidateText("摘要", compactionReadCall()))
			return
		}
		close(reviewStarted)
		<-r.Context().Done()
	})
	done := make(chan error, 1)
	go func() { _, err := core.CompactChat(task.ID); done <- err }()
	select {
	case <-reviewStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("review did not start")
	}
	service := testServer[*backend.ChatServer](t, core, backend.ChatServerName)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("compaction did not stop with its chat generation: %v", err)
	}
	loaded, err := core.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, data, _ := loaded.Tree.CurrentCompaction()
	if data != nil || counter.reads.Load() != 0 {
		t.Fatal("canceled compaction committed its candidate or ran restore")
	}
}

func TestChatAutomaticallyCompactsBeforeModelRequest(t *testing.T) {
	core, task, _, _ := useCompactionChat(t)
	task.Tree = chatstruct.CreateTree("automatic")
	task.Tree.Root.PersonaID = "assistant"
	seedCompactionChat(t, core, task, 6, 5000)
	core.Configs.EnablesConfig.Model[0].MaxContext = 65536
	core.Configs.EnablesConfig.Model[0].MaxOutputTokens = 4096
	var requests atomic.Int32
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		switch requests.Add(1) {
		case 1:
			if request.Messages[0].Content != texts.CompactionSummarize {
				t.Error("full context was sent before automatic compaction")
			}
			qqSSE(w, compactionCandidateText("automatic-summary"))
		case 2:
			qqSSE(w, `{"passed":true,"issues":[]}`)
		case 3:
			text := compactionRequestText(request)
			if !strings.Contains(text, "automatic-summary") || !strings.Contains(text, "latest question") || strings.Contains(text, "old-user-0") {
				t.Error("automatic compaction did not replace the old history")
			}
			qqSSE(w, "final answer")
		default:
			t.Error("unexpected extra request")
			qqSSE(w, "extra")
		}
	})
	if err := core.SendChatMessage(task.ID, "latest question", "high"); err != nil {
		t.Fatal(err)
	}
	loaded, err := core.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := loaded.Tree.CurrentPath()
	reply := path[len(path)-1]
	raw, _ := loaded.Tree.Root.Next[0].GetMessages()
	if requests.Load() != 3 || reply.Compaction == nil || len(reply.Child) != 1 || reply.Child[0].Content != "final answer" || len(raw) != 14 {
		t.Fatal("automatic compaction was not saved on the generating node or changed original history")
	}
}

func TestChatCompactionBetweenToolRoundsDoesNotReplayTools(t *testing.T) {
	core, task, file, counter := useCompactionChat(t)
	task.Tree = chatstruct.CreateTree("tool rounds")
	seedCompactionChat(t, core, task, 4, 4000)
	core.Configs.EnablesConfig.Model[0].MaxContext = 65536
	core.Configs.EnablesConfig.Model[0].MaxOutputTokens = 4096
	content := strings.Repeat("tool-output ", 10000)
	writeFileToolFixture(t, file, content)
	var requests atomic.Int32
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		switch requests.Add(1) {
		case 1:
			qqSSE(w, "", approvalCall("large-read", "ReadFile", map[string]any{"path": []string{"note.txt"}}))
		case 2:
			if request.Messages[0].Content != texts.CompactionSummarize {
				t.Error("large tool result did not trigger compaction")
			}
			qqSSE(w, compactionCandidateText("old-history-summary"))
		case 3:
			qqSSE(w, `{"passed":true,"issues":[]}`)
		case 4:
			calls, results := 0, 0
			for _, message := range request.Messages {
				calls += len(message.ToolCalls)
				if message.Role == "tool" {
					results++
					if message.ToolCallID != "large-read" || message.Content != content {
						t.Error("retained tool output changed")
					}
				}
			}
			if calls != 1 || results != 1 || counter.reads.Load() != 1 {
				t.Error("compaction split or replayed the tool group")
			}
			qqSSE(w, "finished")
		default:
			t.Error("unexpected request")
			qqSSE(w, "extra")
		}
	})
	if err := core.SendChatMessage(task.ID, "read current file", "high"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := core.GetChat(task.ID)
	path, _ := loaded.Tree.CurrentPath()
	reply := path[len(path)-1]
	if requests.Load() != 4 || counter.reads.Load() != 1 || reply.Compaction == nil || len(reply.Child) != 3 || reply.Child[1].Content != content {
		t.Fatal("compaction between tool rounds changed the persisted execution")
	}
}

func TestCompactionPlanCanAdvanceWithinLongToolTurn(t *testing.T) {
	tree := chatstruct.CreateTree("long turn")
	old := tree.Start(api.ChatMessage{Role: "user", Content: "old history", Tokens: 4096})
	oldReply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "old answer", Tokens: 4096})
	old.AddNext(oldReply)
	user := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: "keep working", Tokens: 10})
	oldReply.AddNext(user)
	reply := chatstruct.CreateChatNode()
	user.AddNext(reply)
	for index := range 4 {
		callID := fmt.Sprintf("read-%d", index)
		reply.AddMessage(api.ChatMessage{Role: "assistant", ToolCalls: []api.ToolCall{{ID: callID, Name: "ReadFile", Arguments: "{}"}}, Tokens: 10})
		reply.AddMessage(api.ChatMessage{Role: "tool", ToolCallID: callID, Content: strings.Repeat("data ", 1000), Tokens: 4000})
	}
	reply.Compaction = &chatstruct.CompactionData{
		Start: chatstruct.MessagePosition{NodeID: user.ID}, Summary: "previous-summary",
	}
	plan, err := tree.NewCompactionPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Start.NodeID != reply.ID || plan.Start.Index != 4 || plan.PreviousSummary != "previous-summary" {
		t.Fatalf("compaction could not advance inside a long turn: %+v", plan)
	}
	if plan.Recent[0].ToolCalls[0].ID != plan.Recent[1].ToolCallID {
		t.Fatal("long-turn cut separated a call from its result")
	}
	reply.Compaction = &chatstruct.CompactionData{Start: plan.Start, Summary: "new-summary"}
	reply.AddMessage(api.ChatMessage{Role: "assistant", ToolCalls: []api.ToolCall{{ID: "pending", Name: "ReadFile", Arguments: "{}"}}, Tokens: 10})
	pending, err := tree.NewCompactionPlan(20)
	if err != nil {
		t.Fatal(err)
	}
	last := pending.Recent[len(pending.Recent)-1]
	if last.ToolCalls[0].ID != "pending" {
		t.Fatal("pending call was compacted away")
	}
}

func TestChatCompactionInsideFirstToolTurn(t *testing.T) {
	core, task, file, counter := useCompactionChat(t)
	task.Tree = chatstruct.CreateTree("first tool turn")
	persistCompactionChat(t, core, task)
	core.Configs.EnablesConfig.Model[0].MaxContext = 65536
	core.Configs.EnablesConfig.Model[0].MaxOutputTokens = 4096
	content := strings.Repeat("file-content ", 8300)
	writeFileToolFixture(t, file, content)
	var requests atomic.Int32
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		switch index := requests.Add(1); index {
		case 1, 2:
			qqSSE(w, "", approvalCall(fmt.Sprintf("read-%d", index), "ReadFile", map[string]any{"path": []string{"note.txt"}}))
		case 3:
			if request.Messages[0].Content != texts.CompactionSummarize {
				t.Error("long initial tool turn was not compacted")
			}
			qqSSE(w, compactionCandidateText("用户要求连续读取，第一次读取已完成；继续处理第二次读取的结果。"))
		case 4:
			qqSSE(w, `{"passed":true,"issues":[]}`)
		case 5:
			calls, results := 0, 0
			for _, message := range request.Messages {
				calls += len(message.ToolCalls)
				if message.Role == "tool" {
					results++
					if message.ToolCallID != "read-2" {
						t.Error("already compacted tool output remained in the request")
					}
				}
			}
			if calls != 1 || results != 1 || counter.reads.Load() != 2 {
				t.Error("initial turn compaction split or re-executed a tool group")
			}
			qqSSE(w, "done")
		default:
			t.Errorf("unexpected request %d", index)
			qqSSE(w, "extra")
		}
	})
	if err := core.SendChatMessage(task.ID, "连续读取文件后继续处理", "high"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := core.GetChat(task.ID)
	path, _ := loaded.Tree.CurrentPath()
	reply := path[len(path)-1]
	if requests.Load() != 5 || reply.Compaction == nil || reply.Compaction.Start.NodeID != reply.ID || reply.Compaction.Start.Index != 2 || len(reply.Child) != 5 {
		t.Fatal("initial tool turn did not save a message-level compaction boundary")
	}
}

func TestCompactChatSecondPassMergesPreviousSummary(t *testing.T) {
	core, task, _, _ := useCompactionChat(t)
	var summaries atomic.Int32
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if request.Messages[0].Content == texts.CompactionReview {
			qqSSE(w, `{"passed":true,"issues":[]}`)
			return
		}
		index := summaries.Add(1)
		if index == 2 {
			var source struct {
				PreviousSummary string                      `json:"previousSummary"`
				History         []chatstruct.CompactionTurn `json:"history"`
			}
			input, _ := request.Messages[1].Content.(string)
			if json.Unmarshal([]byte(input), &source) != nil || source.PreviousSummary != "first-summary" {
				t.Error("second compaction omitted its previous checkpoint")
			}
			if strings.Contains(input, "old-user-0") {
				t.Error("second pass re-read already compacted history")
			}
		}
		qqSSE(w, compactionCandidateText(map[int32]string{1: "first-summary", 2: "merged-summary"}[index]))
	})
	first, err := core.CompactChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldMarker, _, _ := first.Tree.CurrentCompaction()
	path, _ := first.Tree.CurrentPath()
	parent := path[len(path)-1]
	for index := range 3 {
		user := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: fmt.Sprintf("new detail %d ", index) + strings.Repeat("important ", 80), Tokens: 4096})
		reply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "new answer", Tokens: 4096})
		parent.AddNext(user)
		user.AddNext(reply)
		parent = reply
	}
	persistCompactionChat(t, core, first)
	second, err := core.CompactChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	marker, data, _ := second.Tree.CurrentCompaction()
	if marker.ID == oldMarker.ID || data.Summary != "merged-summary" || second.Tree.FindNode(oldMarker.ID).Compaction.Summary != "first-summary" {
		t.Fatal("new checkpoint overwrote an ancestor or was not selected")
	}
}

func TestCompactChatSaveFailureRollsBackBeforeRestore(t *testing.T) {
	for _, target := range []string{"chat", "index"} {
		t.Run(target, func(t *testing.T) {
			core, task, _, counter := useCompactionChat(t)
			compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
				if request.Messages[0].Content == texts.CompactionSummarize {
					qqSSE(w, compactionCandidateText("summary", compactionReadCall()))
					return
				}
				name := "index.json"
				if target == "chat" {
					name = task.ID + ".json"
				}
				core.FS = &failChatCommitFS{FilePlatform: core.FS, name: name}
				qqSSE(w, `{"passed":true,"issues":[]}`)
			})
			if _, err := core.CompactChat(task.ID); err == nil {
				t.Fatal("save failure was ignored")
			}
			loaded, err := core.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, data, _ := loaded.Tree.CurrentCompaction()
			if data != nil || counter.reads.Load() != 0 {
				t.Fatal("failed save left an active checkpoint or executed restore")
			}
		})
	}
}

func TestCompactionRestoreResultsAreBoundedAndReportReadFailures(t *testing.T) {
	core, task, file, _ := useCompactionChat(t)
	writeFileToolFixture(t, file, strings.Repeat("中文 \"escaped\"\r\n", 15000))
	compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if request.Messages[0].Content == texts.CompactionReview {
			qqSSE(w, `{"passed":true,"issues":[]}`)
			return
		}
		missing := chatstruct.CompactionRestoreCall{Tool: "ReadFile", Arguments: json.RawMessage(`{"path":["missing.txt"]}`), Purpose: "检查缺失文件"}
		qqSSE(w, compactionCandidateText("summary", compactionReadCall(), missing))
	})
	compressed, err := core.CompactChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, data, _ := compressed.Tree.CurrentCompaction()
	if len(data.Results) != 4 || !strings.Contains(data.Results[1].Content, texts.CompactionTruncated) || data.Results[3].ToolError == "" {
		t.Fatal("restore did not bound large output or report a missing file")
	}
	body, _ := json.Marshal(data.Results)
	if api.EstimateTextTokens(string(body)) > 4096 || !json.Valid(body) {
		t.Fatal("restore result JSON exceeded its budget or lost UTF-8 validity")
	}
}

func TestCompactionRestoreEscapingAndErrorMetadataStayWithinBudget(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		content string
		readErr error
	}{
		{name: "JSON escaping", content: strings.Repeat("<>&\x00", 15000)},
		{name: "large error", readErr: errors.New(strings.Repeat("读取错误<>&", 3000))},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			core, task, file, counter := useCompactionChat(t)
			writeFileToolFixture(t, file, fixture.content)
			counter.readErr = fixture.readErr
			compactionHTTPModel(t, core, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
				if request.Messages[0].Content == texts.CompactionReview {
					qqSSE(w, `{"passed":true,"issues":[]}`)
					return
				}
				qqSSE(w, compactionCandidateText("summary", compactionReadCall()))
			})
			compressed, err := core.CompactChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := compressed.Tree.ContextMessages()
			if err != nil {
				t.Fatal(err)
			}
			last := messages[len(messages)-1]
			if !strings.HasPrefix(last.Content, texts.CompactionRestored) || api.MessageTokens(last) > 4096 {
				t.Fatal("serialized restore content and metadata exceeded the request budget")
			}
			_, data, _ := compressed.Tree.CurrentCompaction()
			if len(data.Results) != 2 || fixture.readErr != nil && data.Results[1].ToolError == "" {
				t.Fatal("bounded restore lost the call/result pair or its error")
			}
		})
	}
}

func TestQQAutomaticCompactionUsesMemoryOnlyToolsAndCountsUsage(t *testing.T) {
	var summaries, reviews, normal atomic.Int32
	f := useQQChat(t, map[string]string{"groupContext": "false"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		switch request.Messages[0].Content {
		case texts.CompactionSummarize:
			summaries.Add(1)
			var source struct {
				RestoreTools []api.Tool `json:"restoreTools"`
			}
			input, _ := request.Messages[1].Content.(string)
			if err := json.Unmarshal([]byte(input), &source); err != nil {
				t.Error(err)
			}
			for _, tool := range source.RestoreTools {
				if tool.Name == "ReadFile" || tool.Name == "SearchFiles" || tool.Name == "ListDirectory" {
					t.Error("normal QQ compaction exposed non-memory restore tools")
				}
			}
			qqSSEUsage(w, compactionCandidateText("qq-summary"), &api.TokenUsage{PromptTokens: 4000, CompletionTokens: 100, TotalTokens: 4100,
				PromptTokensDetails: &api.PromptTokenDetails{CachedTokens: intPointer(1000)}}, 1)
		case texts.CompactionReview:
			reviews.Add(1)
			qqSSEUsage(w, `{"passed":true,"issues":[]}`, &api.TokenUsage{PromptTokens: 4500, CompletionTokens: 20, TotalTokens: 4520,
				PromptTokensDetails: &api.PromptTokenDetails{CachedTokens: intPointer(500)}}, 1)
		default:
			index := normal.Add(1)
			if index > 1 && !strings.Contains(compactionRequestText(request), "qq-summary") {
				t.Error("QQ continuation omitted its new summary")
			}
			qqSSEUsage(w, "完成", &api.TokenUsage{PromptTokens: 1000, CompletionTokens: 10, TotalTokens: 1010,
				PromptTokensDetails: &api.PromptTokenDetails{CachedTokens: intPointer(0)}}, 1)
		}
	})
	f.exchange(t, "private", 30003, 0, "first", false)
	f.waitTokenStats(t, 1)
	record := f.contexts(t)[0]
	session := readQQSession(t, f, record.ID)
	session.Tree = chatstruct.CreateTree("QQ long context")
	parent := ""
	for index := range 6 {
		user := chatstruct.CreateChatNode(api.ChatMessage{Role: "user", Content: fmt.Sprintf("QQ old %d ", index) + strings.Repeat("history ", 60), Tokens: 5000})
		reply := chatstruct.CreateChatNode(api.ChatMessage{Role: "assistant", Content: "QQ reply", Tokens: 5000})
		session.Tree.AddNext(parent, user)
		user.AddNext(reply)
		parent = reply.ID
	}
	body, _ := json.Marshal(session)
	sessionPath := filepath.Join(f.core.Paths.Data, "ext", "clients", f.clientID, "qq", f.extensionID, "sessions", record.ID+".json")
	if err := os.WriteFile(sessionPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	q, _ := f.core.Servers.Get[*backend.QQServer](backend.QQServerName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
	f.core.Configs.EnablesConfig.Model[0].MaxContext = 65536
	f.core.Configs.EnablesConfig.Model[0].MaxOutputTokens = 4096
	f.restartQQ(t)
	f.message(t, "private", 30003, 0, "continue QQ", false)
	if reply := qqPlainReply(f.reply(t)); reply != "完成" {
		t.Fatalf("compaction generated an extra QQ message: %s", reply)
	}
	stats := f.waitTokenStats(t, 4)
	if summaries.Load() != 1 || reviews.Load() != 1 || normal.Load() != 2 || stats.TotalTokens != 10640 || stats.CachedPromptTokens != 1500 {
		t.Fatalf("compaction and review were not included in QQ totals: %+v", stats)
	}
	record = f.contexts(t)[0]
	if record.ContextTokens == nil || *record.ContextTokens != 1010 {
		t.Fatal("summary or review overwrote the active QQ context length")
	}
	saved := readQQSession(t, f, record.ID)
	_, data, err := saved.Tree.CurrentCompaction()
	if err != nil || data == nil || data.Summary != "qq-summary" {
		t.Fatal("QQ compaction was not saved through its internal storage", err)
	}
}
