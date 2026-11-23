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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	memory "github.com/caliniya/caf/Memory"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	mainTools "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
	interaction "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool/InteractionTool"
	memoryTools "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MemoryTool"
	backend "github.com/caliniya/caf/core/Backend"
)

type approvalProbeTool struct {
	agenttool.ToolMeta
	executed atomic.Int32
}

func (p *approvalProbeTool) Load() {
	p.ToolMeta = agenttool.ToolMeta{ID: "ApprovalProbe", Name: "另一个写入工具", Enabled: true, Permission: agenttool.PermissionFullAccess}
}
func (p *approvalProbeTool) Meta() *agenttool.ToolMeta { return &p.ToolMeta }
func (p *approvalProbeTool) Fun(*agenttool.ToolContext, map[string]any) (string, error) {
	p.executed.Add(1)
	return "executed", nil
}

func approvalCore(t *testing.T) (*backend.Core, *approvalProbeTool) {
	t.Helper()
	useToolRegistry(t)
	previous := backend.Platform
	t.Cleanup(func() { backend.Platform = previous })
	probe := &approvalProbeTool{}
	probe.Load()
	set := mainTools.InitMainTool()
	set.Get("文件操作工具").ListName()[probe.ID] = probe
	if err := agenttool.Add(set); err != nil {
		t.Fatal(err)
	}
	if err := agenttool.Add(memoryTools.InitMemoryTool()); err != nil {
		t.Fatal(err)
	}
	return useChatTaskCore(t), probe
}

func approvalCall(id, name string, args map[string]any) api.ToolCall {
	body, _ := json.Marshal(args)
	return api.ToolCall{ID: id, Name: name, Arguments: string(body)}
}

func approvalModel(t *testing.T, core *backend.Core, rounds ...func(api.ChatCompletionsRequest) []api.ToolCall) *atomic.Int32 {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request api.ChatCompletionsRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		index := int(requests.Add(1)) - 1
		w.Header().Set("Content-Type", "text/event-stream")
		if index >= len(rounds) || rounds[index] == nil {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		calls := rounds[index](request)
		if len(calls) == 0 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		values := make([]any, 0, len(calls))
		for i, call := range calls {
			values = append(values, map[string]any{"index": i, "id": call.ID, "type": "function", "function": map[string]string{"name": call.Name, "arguments": call.Arguments}})
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": values}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
	}))
	t.Cleanup(server.Close)
	core.Configs.EnablesConfig.ServerAddress = server.URL
	return requests
}

func approvalEvents(t *testing.T, fn func(map[string]string, api.ToolApproval)) {
	t.Helper()
	previous := backend.Emit
	t.Cleanup(func() { backend.Emit = previous })
	backend.Emit = func(event string, data ...any) {
		if event != backend.EventChatTool {
			return
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(data[0].(string)), &payload); err != nil {
			t.Error(err)
			return
		}
		if payload["requestId"] == "" || payload["approval"] == "" {
			return
		}
		var view api.ToolApproval
		if err := json.Unmarshal([]byte(payload["approval"]), &view); err != nil {
			t.Error(err)
			return
		}
		fn(payload, view)
	}
}

func TestAutomaticToolApprovalChoicesAndScopes(t *testing.T) {
	for _, choice := range []string{backend.ApprovalReject, backend.ApprovalOnce, backend.ApprovalWorkspace, backend.ApprovalTool, backend.ApprovalElevate} {
		t.Run(choice, func(t *testing.T) {
			core, probe := approvalCore(t)
			project, outside := t.TempDir(), t.TempDir()
			workspace, err := core.CreateWorkspace("审批项目", project)
			if err != nil {
				t.Fatal(err)
			}
			task, err := core.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatWorkspace(task.ID, workspace.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatToolConfig(task.ID, `{"permission":1}`); err != nil {
				t.Fatal(err)
			}
			first, second, external := filepath.Join(project, "first.txt"), filepath.Join(project, "second.txt"), filepath.Join(outside, "external.txt")
			calls := []api.ToolCall{
				approvalCall("first", "WriteFile", map[string]any{"path": []string{first}, "content": "first"}),
				approvalCall("second", "WriteFile", map[string]any{"path": []string{second}, "content": "second"}),
				approvalCall("external", "WriteFile", map[string]any{"path": []string{external}, "content": "external"}),
				approvalCall("other-tool", probe.ID, nil),
			}
			calls[3].Arguments = "{}"
			var approvals atomic.Int32
			approvalEvents(t, func(payload map[string]string, view api.ToolApproval) {
				count := approvals.Add(1)
				if count == 1 {
					if _, err := os.Stat(first); !os.IsNotExist(err) {
						t.Error("工具在批准前写入了文件")
					}
					if !view.WorkspaceAllowed || view.WorkspaceID != workspace.ID || view.HigherPermission != 2 {
						t.Errorf("工作区审批范围错误：%+v", view)
					}
				}
				if view.CallID == "external" && view.WorkspaceAllowed {
					t.Error("工作区外文件不能获得工作区授权")
				}
				if _, err := core.SubmitToolApproval("other-chat", payload["requestId"], backend.ApprovalOnce); err == nil {
					t.Error("其他对话可以批准请求")
				}
				if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], "invalid"); err == nil {
					t.Error("无效选择被接受")
				}
				selected := backend.ApprovalReject
				if count == 1 {
					selected = choice
				}
				if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], selected); err != nil {
					t.Error(err)
				}
				if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], selected); err == nil {
					t.Error("审批被重复提交")
				}
			})
			approvalModel(t, core, func(request api.ChatCompletionsRequest) []api.ToolCall {
				for _, tool := range request.Tools {
					if tool.Function.Name == agenttool.RequestToolExecutionID {
						t.Error("普通模式不应声明申请工具")
					}
				}
				return calls
			})
			if err := core.SendChatMessage(task.ID, "执行文件操作", "high"); err != nil {
				t.Fatal(err)
			}
			for _, expected := range []struct {
				path   string
				exists bool
			}{
				{first, choice != backend.ApprovalReject},
				{second, choice == backend.ApprovalWorkspace || choice == backend.ApprovalTool || choice == backend.ApprovalElevate},
				{external, choice == backend.ApprovalTool},
			} {
				_, err := os.Stat(expected.path)
				if (err == nil) != expected.exists {
					t.Errorf("授权范围错误：%s / %v", expected.path, err)
				}
			}
			if probe.executed.Load() != 0 {
				t.Error("单工具授权或自动编辑权限放行了另一个工具")
			}
			loaded, err := core.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if choice == backend.ApprovalElevate {
				if loaded.ToolConfig.Permission != agenttool.PermissionAutoEdit {
					t.Error("权限提升未保存，或提升到了不必要的档位")
				}
			} else if loaded.ToolConfig.Permission != agenttool.PermissionReadOnly {
				t.Error("批准单个工具更改了整个对话权限")
			}
			grant := loaded.ToolConfig.AllowedTools["WriteFile"]
			if choice == backend.ApprovalWorkspace && (grant.Scope != "workspace" || grant.WorkspaceID != workspace.ID) {
				t.Fatal("工作区授权未保存")
			}
			if choice == backend.ApprovalTool && (grant.Scope != "tool" || grant.WorkspaceID != "") {
				t.Fatal("工具授权未保存")
			}
			messages := loaded.Tree.Root.Next[0].Next[0].Child
			if messages[0].ToolCalls[0].Approval.Decision != choice {
				t.Fatal("审批决定没有保存到调用历史")
			}
			protocol := &api.ChatCompletionAPI{}
			wire, err := protocol.EncodeRequest(api.ChatRequest{Model: "test", Messages: messages})
			if err != nil || strings.Contains(wire, `"approval"`) {
				t.Fatal("界面的审批记录被发送给模型")
			}
		})
	}
}

func TestStrictModeRequestsTheOriginalCallExactlyOnce(t *testing.T) {
	core, _ := approvalCore(t)
	project := t.TempDir()
	workspace, err := core.CreateWorkspace("严格模式项目", project)
	if err != nil {
		t.Fatal(err)
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatWorkspace(task.ID, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatToolConfig(task.ID, `{"permission":1,"strictMode":true,"sets":{"主工具集":{"kits":{"交互工具":false}}}}`); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(project, "original.txt")
	var approvals atomic.Int32
	approvalEvents(t, func(payload map[string]string, view api.ToolApproval) {
		approvals.Add(1)
		if payload["callId"] != "request-original" || view.CallID != "blocked-original" || view.ToolID != "WriteFile" {
			t.Errorf("申请关联了错误的调用：%+v / %+v", payload, view)
		}
		if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], backend.ApprovalOnce); err != nil {
			t.Error(err)
		}
	})
	approvalModel(t, core,
		func(request api.ChatCompletionsRequest) []api.ToolCall {
			found := false
			for _, tool := range request.Tools {
				if tool.Function.Name == agenttool.RequestToolExecutionID {
					found = true
				}
			}
			if !found {
				t.Error("严格模式没有自动启用申请工具")
			}
			return []api.ToolCall{approvalCall("blocked-original", "WriteFile", map[string]any{"path": []string{target}, "content": "原始正文"})}
		},
		func(request api.ChatCompletionsRequest) []api.ToolCall {
			if approvals.Load() != 0 {
				t.Error("严格模式在模型申请之前弹出了审批")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Error("严格模式提前执行了越权工具")
			}
			last := request.Messages[len(request.Messages)-1]
			if last.ToolCallID != "blocked-original" || !strings.Contains(fmt.Sprint(last.Content), "RequestToolExecution") {
				t.Error("权限拒绝没有告诉模型如何申请原调用")
			}
			return []api.ToolCall{
				approvalCall("request-invalid", agenttool.RequestToolExecutionID, map[string]any{"call_id": "past-chat-call"}),
				approvalCall("request-original", agenttool.RequestToolExecutionID, map[string]any{"call_id": "blocked-original", "content": "伪造的新正文"}),
				approvalCall("request-replay", agenttool.RequestToolExecutionID, map[string]any{"call_id": "blocked-original"}),
			}
		},
	)
	if err := core.SendChatMessage(task.ID, "写入文件", "high"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "原始正文" || approvals.Load() != 1 {
		t.Fatalf("调用参数被替换或重复执行：%s / %v / %d", data, err, approvals.Load())
	}
}

func TestApprovalWaitingDoesNotConsumeExecutionTimeoutAndCancellationStopsWrites(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			core, _ := approvalCore(t)
			target := filepath.Join(t.TempDir(), "external.txt")
			task, err := core.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
				return []api.ToolCall{approvalCall("waiting", "WriteFile", map[string]any{"path": []string{target}, "content": "approved", "timeout_seconds": 0.2})}
			})
			pending := make(chan map[string]string, 1)
			approvalEvents(t, func(payload map[string]string, view api.ToolApproval) { pending <- payload })
			done := make(chan error, 1)
			go func() { done <- core.SendChatMessage(task.ID, "等待审批", "high") }()
			var request map[string]string
			select {
			case request = <-pending:
			case <-time.After(3 * time.Second):
				t.Fatal("未收到审批请求")
			}
			if _, err := core.SubmitToolApproval(task.ID, request["requestId"], backend.ApprovalWorkspace); err == nil {
				t.Error("工作区外操作接受了工作区授权")
			}
			if cancel {
				server := testServer[*backend.ChatServer](t, core, backend.ChatServerName)
				ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				if err := server.Close(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				<-time.After(300 * time.Millisecond)
				if _, err := core.SubmitToolApproval(task.ID, request["requestId"], backend.ApprovalOnce); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if cancel && !errors.Is(err, context.Canceled) {
					t.Errorf("取消未结束生成：%v", err)
				}
				if !cancel && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("审批结束后工具未退出")
			}
			_, err = os.Stat(target)
			if (err == nil) == cancel {
				t.Errorf("取消后仍写入或审批等待耗尽执行超时：%v", err)
			}
			if _, err := core.SubmitToolApproval(task.ID, request["requestId"], backend.ApprovalOnce); err == nil {
				t.Error("已结束审批仍可重放")
			}
		})
	}
}

func TestMemoryApprovalKeepsWorkspaceAndEnabledLibraryBoundaries(t *testing.T) {
	for _, choice := range []string{backend.ApprovalWorkspace, backend.ApprovalTool} {
		t.Run(choice, func(t *testing.T) {
			core, _ := approvalCore(t)
			workspace, err := core.CreateWorkspace("项目", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			other, err := core.CreateWorkspace("其他项目", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			server := testServer[*memory.MemoryServer](t, core, memory.MemoryServerName)
			local, err := server.EnsureWorkspace(context.Background(), workspace.ID, workspace.Name)
			if err != nil {
				t.Fatal(err)
			}
			foreign, err := server.EnsureWorkspace(context.Background(), other.ID, other.Name)
			if err != nil {
				t.Fatal(err)
			}
			global := mainMemoryID(t, server)
			task, err := core.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatWorkspace(task.ID, workspace.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatMemoryLibraries(task.ID, []string{local.ID, global, foreign.ID}); err != nil {
				t.Fatal(err)
			}
			if _, err := core.SetChatToolConfig(task.ID, `{"permission":1}`); err != nil {
				t.Fatal(err)
			}
			var approvals atomic.Int32
			approvalEvents(t, func(payload map[string]string, view api.ToolApproval) {
				selected := backend.ApprovalReject
				if approvals.Add(1) == 1 {
					selected = choice
				}
				if view.CallID == "foreign" {
					t.Error("不能写入的其他工作区记忆不应通过审批放行")
				}
				if _, err := core.SubmitToolApproval(task.ID, payload["requestId"], selected); err != nil {
					t.Error(err)
				}
			})
			approvalModel(t, core, func(api.ChatCompletionsRequest) []api.ToolCall {
				calls := []api.ToolCall{}
				for _, target := range []struct{ id, library string }{{"local", local.ID}, {"global", global}, {"foreign", foreign.ID}} {
					calls = append(calls, approvalCall(target.id, "CreateMemory", map[string]any{"recollection_id": target.library, "data": target.id}))
				}
				return calls
			})
			if err := core.SendChatMessage(task.ID, "保存记忆", "high"); err != nil {
				t.Fatal(err)
			}
			for _, expected := range []struct {
				id    string
				count int
			}{{local.ID, 1}, {foreign.ID, 0}, {global, map[bool]int{false: 0, true: 1}[choice == backend.ApprovalTool]}} {
				library, err := server.GetRecollection(context.Background(), expected.id)
				if err != nil || len(library.Data) != expected.count {
					t.Errorf("记忆授权范围被越过：%s / %v", expected.id, err)
				}
			}
			if choice == backend.ApprovalWorkspace && approvals.Load() != 2 {
				t.Error("工作区授权自动放行了全局记忆")
			}
			if choice == backend.ApprovalTool && approvals.Load() != 1 {
				t.Error("单工具授权没有复用")
			}
		})
	}
}

func TestRequestToolExecutionValidationAndStrictSettingPersistence(t *testing.T) {
	tool := &interaction.RequestToolExecution{}
	tool.Load()
	ctx := agenttool.CreateContext("", agenttool.PermissionReadOnly)
	calls := 0
	ctx.RequestToolExecution = func(context.Context, string) (string, error) { calls++; return "approved result", nil }
	for _, args := range []map[string]any{nil, {"call_id": " "}, {"call_id": 4}} {
		if _, err := tool.Fun(ctx, args); err == nil {
			t.Error("申请工具接受了无效调用 ID")
		}
	}
	if result, err := tool.Fun(ctx, map[string]any{"call_id": "call"}); err != nil || result != "approved result" || calls != 1 {
		t.Fatal("申请工具未使用宿主回调")
	}
	core, _ := approvalCore(t)
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, strict := range []bool{true, false} {
		if _, err := core.SetChatToolConfig(task.ID, fmt.Sprintf(`{"strictMode":%t}`, strict)); err != nil {
			t.Fatal(err)
		}
		loaded, err := core.GetChat(task.ID)
		if err != nil || loaded.ToolConfig == nil || loaded.ToolConfig.StrictMode == nil || *loaded.ToolConfig.StrictMode != strict {
			t.Fatal("严格模式或显式关闭没有持久化")
		}
	}
	if _, err := core.SetChatToolConfig(task.ID, `{"permission":4,"allowedTools":{"WriteFile":{"scope":"tool"}}}`); err != nil {
		t.Fatal(err)
	}
	loaded, err := core.SetChatToolConfig(task.ID, `{"permission":1,"allowedTools":{"WriteFile":{"scope":"tool"}}}`)
	if err != nil || len(loaded.ToolConfig.AllowedTools) != 0 {
		t.Fatal("降低权限没有清除旧授权")
	}
	if err := backend.NewToolApprovalServer(core).Start(context.Background()); err == nil {
		t.Error("未注册的审批服务可以使用宿主状态")
	}
}
