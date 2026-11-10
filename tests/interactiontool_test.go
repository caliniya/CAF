package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	maintool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
	interactiontool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool/InteractionTool"
	backend "github.com/caliniya/caf/core/Backend"
)

func TestInteractionToolQuestionValidationAndHost(t *testing.T) {
	kit := interactiontool.InitInteractionTool()
	tool := kit.ListName()["AskUser"]
	if len(kit.List()) != 1 || tool == nil || !kit.Enabled || tool.Meta().DisplayType != "question" || tool.Meta().DisplayContent != "arguments" {
		t.Fatalf("交互工具注册错误：%+v", kit)
	}
	if tool.ToType().Name != "AskUser" || tool.ToType().Parameters["properties"].(map[string]any)["timeout_seconds"] == nil {
		t.Fatal("提问工具必须提供模型参数声明和可选超时")
	}
	if timeout, err := tool.Meta().TimeoutFor(nil); err != nil || timeout != 30*time.Minute {
		t.Fatalf("提问应允许用户思考后回答：%s，%v", timeout, err)
	}
	ctx := agenttool.CreateContext("", agenttool.PermissionReadOnly)
	calls := 0
	ctx.AskUser = func(_ context.Context, request agenttool.UserQuestion) (string, error) {
		calls++
		if request.Question != "选择方案？" || !reflect.DeepEqual(request.Options, []string{"A", "B"}) {
			t.Errorf("问题或选项丢失：%+v", request)
		}
		return "使用其他方案", nil
	}
	output, err := tool.Fun(ctx, map[string]any{"question": "选择方案？", "options": []any{"A", "B"}})
	if err != nil || output != "使用其他方案" || calls != 1 {
		t.Fatalf("应返回用户自由输入的回答：%q，%v", output, err)
	}
	for _, args := range []map[string]any{
		{}, {"question": " "}, {"question": 1},
		{"question": "问题", "options": nil}, {"question": "问题", "options": []any{}},
		{"question": "问题", "options": []any{"A", 1}}, {"question": "问题", "options": []string{" "}},
		{"question": "问题", "options": []string{"A", " A "}},
	} {
		if _, err := tool.Fun(ctx, args); err == nil {
			t.Errorf("应拒绝无效参数：%v", args)
		}
	}
	if calls != 1 {
		t.Fatal("无效参数不能触发交互")
	}
	if _, err := tool.Fun(nil, map[string]any{"question": "问题"}); err == nil {
		t.Fatal("无交互宿主时应返回错误")
	}
	cancelContext, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Context = cancelContext
	ctx.AskUser = func(ctx context.Context, _ agenttool.UserQuestion) (string, error) { return "", ctx.Err() }
	if _, err := tool.Fun(ctx, map[string]any{"question": "问题"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("等待交互应响应取消：%v", err)
	}
}

func TestChatUserQuestionAnswerAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprintf("timeout=%v", timeout), func(t *testing.T) {
			useToolRegistry(t)
			if err := agenttool.Add(maintool.InitMainTool()); err != nil {
				t.Fatal(err)
			}
			instance := useChatTaskCore(t)
			requests := 0
			arguments := `{"question":"选择方案？","options":["A","B"],"timeout_seconds":0.2}`
			if !timeout {
				arguments = `{"question":"选择方案？","options":["A","B"]}`
			}
			response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index": 0, "id": "ask-call", "type": "function", "function": map[string]string{"name": "AskUser", "arguments": arguments},
				}}}, "finish_reason": "tool_calls",
			}}})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var request apiImpl.ChatCompletionsRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if requests > 1 {
					last := request.Messages[len(request.Messages)-1]
					content, _ := last.Content.(string)
					if last.Role != "tool" || last.ToolCallID != "ask-call" || (!timeout && content != "自定义回答") || (timeout && !strings.Contains(content, "执行超时")) {
						t.Errorf("模型应收到用户回答或超时结果：%+v", last)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if requests == 1 {
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", response)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			instance.Configs.EnablesConfig.ServerAddress = server.URL
			var mu sync.Mutex
			var events []map[string]string
			previousEmit := backend.Emit
			t.Cleanup(func() { backend.Emit = previousEmit })
			backend.Emit = func(event string, data ...any) {
				if event != backend.EventChatTool {
					return
				}
				var payload map[string]string
				if err := json.Unmarshal([]byte(data[0].(string)), &payload); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				events = append(events, payload)
				mu.Unlock()
				if payload["event"] != "update" || timeout {
					return
				}
				chatID, requestID := payload["chatId"], payload["requestId"]
				if err := instance.SubmitUserAnswer("other-chat", requestID, "错误回答"); err == nil {
					t.Error("不能回答其他会话的问题")
				}
				if err := instance.SubmitUserAnswer(chatID, requestID, " "); err == nil {
					t.Error("不能提交空回答")
				}
				if err := instance.SubmitUserAnswer(chatID, requestID, "自定义回答"); err != nil {
					t.Error(err)
				}
				if err := instance.SubmitUserAnswer(chatID, requestID, "重复回答"); err == nil {
					t.Error("不能重复提交回答")
				}
			}
			task, err := instance.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := instance.SendChatMessage(task.ID, "选择方案", "high"); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			status := "done"
			if timeout {
				status = "fail"
			}
			if len(events) != 3 || events[0]["event"] != "start" || events[1]["event"] != "update" || events[2]["event"] != status {
				t.Fatalf("问题应经历开始、等待回答、结束：%+v", events)
			}
			if events[1]["requestId"] == "" || events[1]["displayType"] != "question" || !strings.Contains(events[1]["content"], "选择方案？") {
				t.Fatalf("问题展示信息缺失：%+v", events[1])
			}
			if err := instance.SubmitUserAnswer(task.ID, events[1]["requestId"], "过期回答"); err == nil {
				t.Fatal("已结束的问题不能回答")
			}
			loaded, err := instance.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			children := loaded.Tree.Root.Next[0].Next[0].Child
			if len(children) != 3 || children[0].ToolCalls[0].DisplayType != "question" || children[0].ToolCalls[0].DisplayContent != "arguments" || children[1].ToolCallID != "ask-call" {
				t.Fatalf("问题和回答应保存到历史：%+v", children)
			}
			if (!timeout && children[1].Content != "自定义回答") || (timeout && children[1].ToolError == "") {
				t.Fatalf("历史回答或失败原因丢失：%+v", children[1])
			}
			if requests != 2 {
				t.Fatalf("回答后模型应继续生成，实际请求 %d 次", requests)
			}
		})
	}
}
