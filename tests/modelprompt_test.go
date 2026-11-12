package Tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

func checkChatModelInfo(t *testing.T, request apiImpl.ChatCompletionsRequest, model apiImpl.Model, effort string) {
	t.Helper()
	count := 0
	seenUser := false
	for _, message := range request.Messages {
		seenUser = seenUser || message.Role == "user"
		content, _ := message.Content.(string)
		if !strings.HasPrefix(content, "以下是当前使用的模型信息") {
			continue
		}
		count++
		if message.Role != "system" || seenUser {
			t.Error("模型信息应以 system 消息放在对话历史之前")
		}
		_, data, _ := strings.Cut(content, "\n")
		var gotModel apiImpl.Model
		var params struct {
			ReasoningEffort string `json:"reasoning_effort"`
		}
		if err := json.Unmarshal([]byte(data), &gotModel); err != nil {
			t.Error(err)
			continue
		}
		if err := json.Unmarshal([]byte(data), &params); err != nil {
			t.Error(err)
			continue
		}
		if !reflect.DeepEqual(gotModel, model) || params.ReasoningEffort != effort {
			t.Errorf("模型信息应匹配当前模型和本次思考强度：%s", data)
		}
		if strings.Contains(content, "secret-key") || strings.Contains(content, "APIKey") || strings.Contains(content, "ServerAddress") {
			t.Error("模型信息不应包含供应商凭据或服务配置")
		}
	}
	if count != 1 {
		t.Errorf("每次请求应包含一份模型信息，实际 %d", count)
	}
	if request.Model != model.ID || request.ReasoningEffort != effort {
		t.Errorf("请求参数与附带信息不一致：model=%q reasoning_effort=%q", request.Model, request.ReasoningEffort)
	}
}

func TestChatRequestsIncludeCurrentModelInfo(t *testing.T) {
	useToolRegistry(t)
	instance := useChatTaskCore(t)
	model := apiImpl.Model{
		ID: "model-2", Name: "模型 2 \"测试\"", MaxContext: 128000, MaxOutputTokens: 8192,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedLevels: []string{"none", "low", "high", "max"}, DefaultLevel: "low",
	}
	instance.Configs.EnablesConfig.Model = []apiImpl.Model{{ID: "model-1"}, model}
	requests := make(chan apiImpl.ChatCompletionsRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request apiImpl.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	instance.Configs.EnablesConfig.ServerAddress = server.URL
	expectRequest := func(effort, text string) {
		t.Helper()
		select {
		case request := <-requests:
			checkChatModelInfo(t, request, model, effort)
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "user" || last.Content != text {
				t.Errorf("用户消息应保持原内容：%+v", last)
			}
		default:
			t.Fatal("没有收到模型请求")
		}
	}
	// 会话选择第二个模型，供应商默认仍是第一个模型。
	task, err := instance.CreateChat("provider-1", model.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.SendChatMessage(task.ID, "你好", "invalid"); err != nil {
		t.Fatal(err)
	}
	expectRequest("high", "你好")
	loaded, err := instance.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	user := loaded.Tree.Root.Next[0]
	// 重新生成使用刷新后的模型元数据和新的思考强度。
	model.MaxContext = 256000
	instance.Configs.EnablesConfig.Model[1] = model
	if err := instance.RegenerateChatMessage(task.ID, user.Next[0].ID, "low"); err != nil {
		t.Fatal(err)
	}
	expectRequest("low", "你好")
	if err := instance.EditChatMessage(task.ID, user.ID, "修改后的问题", "none"); err != nil {
		t.Fatal(err)
	}
	expectRequest("none", "修改后的问题")
	model = instance.Configs.EnablesConfig.Model[0]
	if _, err := instance.UpdateChatModel(task.ID, "provider-1", model.ID); err != nil {
		t.Fatal(err)
	}
	if err := instance.SendChatMessage(task.ID, "继续", "max"); err != nil {
		t.Fatal(err)
	}
	expectRequest("max", "继续")
	loaded, err = instance.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Tree.Each(func(message *apiImpl.ChatMessage) {
		if message.Role == "system" {
			t.Error("发送时附带的系统信息不应写入对话树")
		}
	}); err != nil {
		t.Fatal(err)
	}
}
