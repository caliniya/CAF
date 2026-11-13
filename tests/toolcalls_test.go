package Tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	maintool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
	backend "github.com/caliniya/caf/core/Backend"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

func TestChatToolUsesIDForRequestsAndNameForDisplay(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%v", fail), func(t *testing.T) {
			useToolRegistry(t)
			if err := agenttool.Add(maintool.InitMainTool()); err != nil {
				t.Fatal(err)
			}
			instance := useChatTaskCore(t)
			path := filepath.Join(instance.Paths.Data, "tool-input.txt")
			if !fail {
				if err := os.WriteFile(path, []byte("工具测试内容"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			arguments, err := json.Marshal(map[string]any{"path": []string{path}})
			if err != nil {
				t.Fatal(err)
			}
			toolResponse, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "call-1", "type": "function",
						"function": map[string]string{"name": "ReadFile", "arguments": string(arguments)},
					}}},
					"finish_reason": "tool_calls",
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				data, readErr := io.ReadAll(r.Body)
				var request apiImpl.ChatCompletionsRequest
				if readErr != nil || json.Unmarshal(data, &request) != nil {
					t.Error("无法解析聊天请求")
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				if len(request.Tools) != len(agenttool.Tools) {
					t.Errorf("工具声明数量错误：%d，期望 %d", len(request.Tools), len(agenttool.Tools))
				}
				requestToolIDs := make(map[string]bool, len(request.Tools))
				for _, tool := range request.Tools {
					if requestToolIDs[tool.Function.Name] || agenttool.Tools[tool.Function.Name] == nil {
						t.Errorf("工具声明应使用唯一的已注册 ID，实际 %+v", tool)
					}
					requestToolIDs[tool.Function.Name] = true
				}
				if !requestToolIDs["ReadFile"] {
					t.Error("工具声明缺少 ReadFile")
				}
				if strings.Contains(string(data), "display_name") || strings.Contains(string(data), "display_type") || strings.Contains(string(data), "display_content") || strings.Contains(string(data), "tool_error") {
					t.Error("展示字段不能发送给模型")
				}
				foundResult := false
				modelInfoCount := 0
				for _, message := range request.Messages {
					if message.Role == "system" {
						content, _ := message.Content.(string)
						if strings.HasPrefix(content, "模型信息：") {
							modelInfoCount++
							if !strings.Contains(content, "模型名称：Model 1") || !strings.Contains(content, "模型 ID：model-1") {
								t.Errorf("模型信息没有使用会话选中的模型：%s", content)
							}
						}
					}
					for _, call := range message.ToolCalls {
						if call.ID != "call-1" || call.Function.Name != "ReadFile" {
							t.Errorf("历史工具调用应保留调用 ID 和工具 ID，实际 %+v", call)
						}
					}
					if message.Role == "tool" {
						foundResult = true
						output, _ := message.Content.(string)
						if message.ToolCallID != "call-1" || (!fail && output != "工具测试内容") || (fail && !strings.HasPrefix(output, "工具执行失败：")) {
							t.Errorf("工具结果没有正确回传：%+v", message)
						}
					}
				}
				if modelInfoCount != 1 {
					t.Errorf("每次模型请求（包括工具续请求）应包含一条模型信息，实际 %d", modelInfoCount)
				}
				if requests > 1 && !foundResult {
					t.Error("工具结果应保留在后续请求中")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if requests == 1 {
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", toolResponse)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			instance.Configs.EnablesConfig.ServerAddress = server.URL
			var events []map[string]string
			previousEmit := backend.Emit
			t.Cleanup(func() { backend.Emit = previousEmit })
			backend.Emit = func(event string, data ...any) {
				if event != backend.EventChatTool {
					return
				}
				var payload map[string]string
				if err := json.Unmarshal([]byte(data[0].(string)), &payload); err != nil {
					t.Fatal(err)
				}
				events = append(events, payload)
			}
			task, err := instance.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := instance.SendChatMessage(task.ID, "读取文件", "high"); err != nil {
				t.Fatal(err)
			}
			status := "done"
			if fail {
				status = "fail"
			}
			if len(events) != 2 || events[0]["event"] != "start" || events[1]["event"] != status {
				t.Fatalf("工具状态事件错误：%+v", events)
			}
			if events[0]["displayType"] != "info" || events[0]["displayContent"] != "result" || events[1]["content"] == "" {
				t.Fatalf("文件读取事件应包含展示方式和读取结果：%+v", events)
			}
			for _, event := range events {
				if event["name"] != "读取文件" || event["toolId"] != "ReadFile" || event["callId"] != "call-1" || event["chatId"] != task.ID {
					t.Fatalf("工具事件必须区分展示名、工具 ID 和调用 ID：%+v", event)
				}
			}
			loaded, err := instance.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			call := loaded.Tree.Root.Next[0].Next[0].Child[0].ToolCalls[0]
			if call.Name != "ReadFile" || call.DisplayName != "读取文件" || call.ID != "call-1" || call.DisplayType != "info" || call.DisplayContent != "result" {
				t.Fatalf("历史记录没有保存展示名和 ID：%+v", call)
			}
			if err := instance.SendChatMessage(task.ID, "继续", "high"); err != nil {
				t.Fatal(err)
			}
			if requests != 3 {
				t.Fatalf("模型应收到工具调用、工具结果和下一轮对话三次请求，实际 %d", requests)
			}
		})
	}
}

func TestChatToolDisplayNamesFillAllSavedBranches(t *testing.T) {
	useToolRegistry(t)
	if err := agenttool.Add(maintool.InitMainTool()); err != nil {
		t.Fatal(err)
	}
	instance := useChatTaskCore(t)
	task, err := instance.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	user := task.Tree.Start(apiImpl.ChatMessage{Role: "user", Content: "读取文件"})
	for i, displayName := range []string{"", "保存时的名称"} {
		user.AddNext(chatstruct.CreateChatNode(apiImpl.ChatMessage{
			Role: "assistant", ToolCalls: []apiImpl.ToolCall{
				{ID: fmt.Sprint(i), Name: "ReadFile", Arguments: "{}", DisplayName: displayName},
				{ID: "unknown", Name: "unknown_tool", Arguments: "{}"},
			},
		}))
	}
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance.Paths.Data, "chats", task.ID+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := instance.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"读取文件", "保存时的名称"} {
		calls := loaded.Tree.Root.Next[0].Next[i].Child[0].ToolCalls
		if calls[0].Name != "ReadFile" || calls[0].DisplayName != expected || calls[1].DisplayName != "unknown_tool" {
			t.Fatalf("第 %d 个分支的展示名称恢复错误：%+v", i, calls)
		}
	}
}
