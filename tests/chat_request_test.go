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
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

func TestChatTreeRequestIncludesModelParameters(t *testing.T) {
	tree := chatstruct.CreateTree("test")
	user := tree.Start(apiImpl.ChatMessage{Role: "user", Content: "question"})
	user.AddNext(chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "other branch"}))
	user.AddNext(chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "selected answer"}))
	before, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		model   apiImpl.Model
		content []string
	}{
		{
			name:    "known limits",
			model:   apiImpl.Model{ID: "model-1", Name: "Model One", MaxContext: 32768, MaxOutputTokens: 4096},
			content: []string{"模型名称：Model One", "模型 ID：model-1", "context_window）：32768 tokens", "max_output_tokens）：4096 tokens"},
		},
		{
			name:    "missing name and limits",
			model:   apiImpl.Model{ID: "model-2", MaxOutputTokens: -1},
			content: []string{"模型名称：model-2", "模型 ID：model-2", "context_window）：未提供", "max_output_tokens）：未提供"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for range 2 {
				request, err := tree.ToChatRequest(tc.model)
				if err != nil {
					t.Fatal(err)
				}
				if request.Model != tc.model.ID || len(request.Messages) != 3 || request.Messages[0].Role != "system" {
					t.Fatalf("request lost model metadata or branch history: %+v", request)
				}
				for _, content := range tc.content {
					if !strings.Contains(request.Messages[0].Content, content) {
						t.Errorf("model information is missing %q: %s", content, request.Messages[0].Content)
					}
				}
				if request.Messages[1].Content != "question" || request.Messages[2].Content != "selected answer" {
					t.Fatalf("request included the wrong branch: %+v", request.Messages)
				}
				if request.MaxTokens != 0 {
					t.Fatal("model metadata must not change the API generation limit")
				}
			}
		})
	}
	after, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("request generation must not persist model information in the tree")
	}
}

func TestChatTreeRequestRejectsEmptyOrInvalidHistory(t *testing.T) {
	emptyNode := chatstruct.CreateTree("test")
	emptyNode.Start(apiImpl.ChatMessage{})
	emptyNode.Root.Next[0].Child = nil
	invalid := chatstruct.CreateTree("test")
	invalid.Start(apiImpl.ChatMessage{Role: "user", Content: "question"})
	invalid.Root.Choice = 2
	cycle := chatstruct.CreateTree("test")
	node := cycle.Start(apiImpl.ChatMessage{Role: "user", Content: "question"})
	node.AddNext(node)
	for name, tree := range map[string]*chatstruct.ChatTree{
		"nil": nil, "uninitialized": {}, "empty": chatstruct.CreateTree("test"),
		"empty node": emptyNode, "invalid choice": invalid, "cycle": cycle,
	} {
		t.Run(name, func(t *testing.T) {
			if request, err := tree.ToChatRequest(apiImpl.Model{ID: "model-1"}); err == nil || request != nil {
				t.Fatalf("invalid history should fail before adding system metadata: %+v, %v", request, err)
			}
		})
	}
}

func TestChatRequestsUseCurrentModelParameters(t *testing.T) {
	useToolRegistry(t)
	instance := useChatTaskCore(t)
	models := []apiImpl.Model{
		{ID: "model-1", Name: "Model One", MaxContext: 32768, MaxOutputTokens: 4096},
		{ID: "model-2", Name: "Model Two", MaxContext: 8192, MaxOutputTokens: 1024},
	}
	instance.Configs.EnablesConfig.Model = models
	var requests []apiImpl.ChatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request apiImpl.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	instance.Configs.EnablesConfig.ServerAddress = server.URL
	task, err := instance.CreateChat("provider-1", "model-2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.SendChatMessage(task.ID, "question", "high"); err != nil {
		t.Fatal(err)
	}
	loaded, err := instance.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	user := loaded.Tree.Root.Next[0]
	if err := instance.RegenerateChatMessage(task.ID, user.Next[0].ID, "low"); err != nil {
		t.Fatal(err)
	}
	if err := instance.EditChatMessage(task.ID, user.ID, "edited", "none"); err != nil {
		t.Fatal(err)
	}
	if _, err := instance.UpdateChatModel(task.ID, "provider-1", "model-1"); err != nil {
		t.Fatal(err)
	}
	if err := instance.SendChatMessage(task.ID, "continue", "max"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 4 {
		t.Fatalf("expected send, regenerate, edit and model switch requests, got %d", len(requests))
	}
	for i, expected := range []struct {
		model   apiImpl.Model
		effort  string
		history []string
	}{
		{models[1], "high", []string{"question"}},
		{models[1], "low", []string{"question"}},
		{models[1], "none", []string{"edited"}},
		{models[0], "max", []string{"edited", "reply", "continue"}},
	} {
		request := requests[i]
		if request.Model != expected.model.ID || request.ReasoningEffort != expected.effort {
			t.Errorf("request %d lost the selected model or reasoning effort: %+v", i, request)
		}
		var history []string
		metadataCount := 0
		for _, message := range request.Messages {
			content, _ := message.Content.(string)
			if message.Role != "system" {
				history = append(history, content)
				continue
			}
			if strings.HasPrefix(content, "模型信息：") {
				metadataCount++
				for _, field := range []string{
					expected.model.Name, expected.model.ID,
					fmt.Sprintf("context_window）：%d tokens", expected.model.MaxContext),
					fmt.Sprintf("max_output_tokens）：%d tokens", expected.model.MaxOutputTokens),
				} {
					if !strings.Contains(content, field) {
						t.Errorf("request %d used stale model parameters: %s", i, content)
					}
				}
			}
		}
		if metadataCount != 1 || !reflect.DeepEqual(history, expected.history) {
			t.Errorf("request %d duplicated metadata or included the wrong branch: metadata=%d, history=%v", i, metadataCount, history)
		}
	}
	loaded, err = instance.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Tree.Each(func(message *apiImpl.ChatMessage) {
		if message.Role == "system" {
			t.Error("request metadata should not be saved in the conversation history")
		}
	}); err != nil {
		t.Fatal(err)
	}
}
