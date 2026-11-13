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
	filetool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool/FileTool"
	backend "github.com/caliniya/caf/core/Backend"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

func TestChatToolRejectsDisabledTools(t *testing.T) {
	for _, tc := range []struct {
		name             string
		chatEnabled      *bool
		globalEnabled    *bool
		disableSet       bool
		disableKit       bool
		disableTool      bool
		invalidArguments bool
		readOnly         bool
		wantDisabled     bool
	}{
		{name: "chat-disabled", chatEnabled: boolPtr(false), wantDisabled: true},
		{name: "global-disabled", globalEnabled: boolPtr(false), wantDisabled: true},
		{name: "set-default-disabled", disableSet: true, wantDisabled: true},
		{name: "kit-default-disabled", disableKit: true, wantDisabled: true},
		{name: "tool-disabled", disableTool: true, wantDisabled: true},
		{name: "chat-disable-over-global-enable", chatEnabled: boolPtr(false), globalEnabled: boolPtr(true), wantDisabled: true},
		{name: "chat-enable-over-global-disable", chatEnabled: boolPtr(true), globalEnabled: boolPtr(false)},
		{name: "global-enable-over-kit-default-disable", globalEnabled: boolPtr(true), disableKit: true},
		{name: "kit-enable-cannot-enable-disabled-tool", chatEnabled: boolPtr(true), disableTool: true, wantDisabled: true},
		{name: "disabled-before-argument-parsing", chatEnabled: boolPtr(false), invalidArguments: true, wantDisabled: true},
		{name: "disabled-before-permission-check", chatEnabled: boolPtr(false), readOnly: true, wantDisabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useToolRegistry(t)
			set := maintool.InitMainTool()
			set.Enabled = !tc.disableSet
			set.Get("文件操作工具").(*filetool.FileTool).Enabled = !tc.disableKit
			set.Get("文件操作工具").ListName()["WriteFile"].Meta().Enabled = !tc.disableTool
			if err := agenttool.Add(set); err != nil {
				t.Fatal(err)
			}
			instance := useChatTaskCore(t)
			globalConfig := &agenttool.ChatToolConfig{}
			globalConfig.SetKit(set.Name, "文件操作工具", tc.globalEnabled)
			instance.Configs.ToolNormalConfig = globalConfig
			chatConfig := &agenttool.ChatToolConfig{Permission: agenttool.PermissionFullAccess}
			if tc.readOnly {
				chatConfig.Permission = agenttool.PermissionReadOnly
			}
			chatConfig.SetKit(set.Name, "文件操作工具", tc.chatEnabled)
			path := filepath.Join(instance.Paths.Data, "tool-output.txt")
			if err := os.WriteFile(path, []byte("原始内容"), 0o600); err != nil {
				t.Fatal(err)
			}
			argumentJSON, err := json.Marshal(map[string]any{"path": []string{path}, "content": "修改后的内容"})
			if err != nil {
				t.Fatal(err)
			}
			arguments := string(argumentJSON)
			if tc.invalidArguments {
				arguments = "{invalid"
			}
			toolResponse, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "write-call", "type": "function",
						"function": map[string]string{"name": "WriteFile", "arguments": arguments},
					}}}, "finish_reason": "tool_calls",
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var request apiImpl.ChatCompletionsRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("无法解析聊天请求：%v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				declared := false
				for _, tool := range request.Tools {
					if tool.Function.Name == "WriteFile" {
						declared = true
					}
				}
				if declared == tc.wantDisabled {
					t.Errorf("工具声明与启停配置不一致：declared=%v disabled=%v", declared, tc.wantDisabled)
				}
				if requests == 2 {
					last := request.Messages[len(request.Messages)-1]
					if last.Role != "tool" || last.ToolCallID != "write-call" {
						t.Errorf("没有回传对应调用的工具结果：%+v", last)
					}
					output, _ := last.Content.(string)
					if tc.wantDisabled && output != "此工具被禁用" {
						t.Errorf("禁用结果必须是原文，实际 %q", output)
					}
					if !tc.wantDisabled && strings.HasPrefix(output, "工具执行失败：") {
						t.Errorf("显式启用的工具不应失败：%s", output)
					}
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
			configJSON, err := json.Marshal(chatConfig)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := instance.SetChatToolConfig(task.ID, string(configJSON)); err != nil {
				t.Fatal(err)
			}
			if err := instance.SendChatMessage(task.ID, "写入文件", "high"); err != nil {
				t.Fatal(err)
			}
			if requests != 2 {
				t.Fatalf("工具结果后应继续对话，实际请求次数 %d", requests)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			wantContent, wantStatus := "修改后的内容", "done"
			if tc.wantDisabled {
				wantContent, wantStatus = "原始内容", "fail"
			}
			if string(content) != wantContent {
				t.Fatalf("工具执行状态不正确，文件内容为 %q，期望 %q", content, wantContent)
			}
			if len(events) != 2 || events[0]["event"] != "start" || events[1]["event"] != wantStatus {
				t.Fatalf("工具状态事件不正确：%+v", events)
			}
			if tc.wantDisabled && events[1]["error"] != "此工具被禁用" {
				t.Fatalf("界面应收到禁用原因：%+v", events[1])
			}
			loaded, err := instance.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			messages := loaded.Tree.Root.Next[0].Next[0].Child
			if len(messages) != 3 || messages[2].Content != "完成" {
				t.Fatalf("对话没有保存工具结果和后续回复：%+v", messages)
			}
			if tc.wantDisabled && (messages[1].Content != "此工具被禁用" || messages[1].ToolError != "此工具被禁用") {
				t.Fatalf("禁用结果没有保存到对话历史：%+v", messages[1])
			}
		})
	}
}

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
