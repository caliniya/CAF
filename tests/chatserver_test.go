package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	maintool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
)

func TestChatServerRegistrationAndInitPreserveInstance(t *testing.T) {
	previous := backend.Platform
	t.Cleanup(func() { backend.Platform = previous })
	backend.Init()
	service, err := backend.Platform.Servers.Get[*backend.ChatServer](backend.ChatServerName)
	if err != nil || service != &backend.Platform.Chat {
		t.Fatalf("registry did not return the Core chat instance: %p / %v", service, err)
	}
	service.ChatList["test"] = chat.ChatTask{Title: "preserved"}
	if err := service.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.Platform.Chat.ChatList["test"].Title != "preserved" {
		t.Fatal("repeated init discarded the existing client cache")
	}
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.ChatList != nil {
		t.Fatal("close did not release the client cache")
	}
	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("repeated close failed: %v", err)
	}
}

func TestChatServerCancelsStreamsPreservesReplyAndRestarts(t *testing.T) {
	for _, shutdown := range []string{"close", "parent cancellation"} {
		t.Run(shutdown, func(t *testing.T) {
			useToolRegistry(t)
			instance := useChatTaskCore(t)
			service := backend.NewChatServer(instance)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := service.Start(ctx); err != nil {
				t.Fatal(err)
			}
			started := make(chan struct{})
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if requests.Add(1) > 1 {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"restarted\"}}]}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial reply\"}}]}\n\n")
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			t.Cleanup(cancel)
			instance.Configs.EnablesConfig.ServerAddress = server.URL
			task, err := instance.CreateChat("", "", "", "")
			if err != nil {
				t.Fatal(err)
			}
			// Wait for the client to consume the partial delta, not only for the server write.
			previousEmit := backend.Emit
			t.Cleanup(func() { backend.Emit = previousEmit })
			deltaSeen := make(chan struct{}, 1)
			backend.Emit = func(event string, _ ...any) {
				if event == backend.EventChatDelta {
					select {
					case deltaSeen <- struct{}{}:
					default:
					}
				}
			}
			done := make(chan error, 1)
			go func() { done <- instance.SendChatMessage(task.ID, "question", "high") }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("stream did not start")
			}
			select {
			case <-deltaSeen:
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("client did not receive the partial reply")
			}
			if err := instance.SendChatMessage(task.ID, "duplicate", "high"); err == nil {
				cancel()
				t.Fatal("concurrent generation in the same chat was accepted")
			}
			if shutdown == "parent cancellation" {
				cancel()
			} else {
				closeCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				if err := service.Close(closeCtx); err != nil {
					cancel()
					t.Fatalf("close failed to drain the request: %v", err)
				}
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("generation did not return cancellation: %v", err)
				}
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("generation kept running after service shutdown")
			}
			loaded, err := instance.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := loaded.Tree.Root.Next[0].GetMessages()
			if err != nil || len(messages) != 2 || messages[1].Content != "partial reply" {
				t.Fatalf("shutdown discarded the partial reply: %+v / %v", messages, err)
			}
			if err := instance.SendChatMessage(task.ID, "blocked", "high"); err == nil {
				t.Fatal("shutdown service accepted new generation")
			}
			if err := service.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := service.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer service.Close(context.Background())
			if err := instance.SendChatMessage(task.ID, "continue", "high"); err != nil {
				t.Fatalf("service could not restart: %v", err)
			}
			if requests.Load() != 2 {
				t.Fatalf("rejected calls unexpectedly reached model API: %d requests", requests.Load())
			}
		})
	}
}

func TestChatServerCloseCancelsUserQuestion(t *testing.T) {
	useToolRegistry(t)
	if err := agenttool.Add(maintool.InitMainTool()); err != nil {
		t.Fatal(err)
	}
	instance := useChatTaskCore(t)
	service := backend.NewChatServer(instance)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"ask\",\"type\":\"function\",\"function\":{\"name\":\"AskUser\",\"arguments\":\"{\\\"question\\\":\\\"选择方案？\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	defer service.Close(context.Background())
	instance.Configs.EnablesConfig.ServerAddress = server.URL
	previousEmit := backend.Emit
	t.Cleanup(func() { backend.Emit = previousEmit })
	question := make(chan map[string]string, 1)
	backend.Emit = func(event string, data ...any) {
		if event != backend.EventChatTool {
			return
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(data[0].(string)), &payload); err != nil {
			t.Error(err)
			return
		}
		if payload["requestId"] != "" {
			question <- payload
		}
	}
	task, err := instance.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- instance.SendChatMessage(task.ID, "ask", "high") }()
	var pending map[string]string
	select {
	case pending = <-question:
	case <-time.After(3 * time.Second):
		t.Fatal("question did not begin waiting")
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := service.Close(closeCtx); err != nil {
		t.Fatalf("close did not cancel the user wait: %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("question cancellation did not end generation: %v", err)
	}
	if err := instance.SubmitUserAnswer(task.ID, pending["requestId"], "late answer"); err == nil {
		t.Fatal("closed question accepted an answer")
	}
	if requests.Load() != 1 {
		t.Fatal("shutdown started an extra model request after cancelling the question")
	}
}

func TestChatServerLifecycleRejectsUnboundAndCancelledContexts(t *testing.T) {
	if err := (&backend.ChatServer{}).Init(context.Background()); err == nil {
		t.Fatal("unbound init succeeded")
	}
	if err := (&backend.ChatServer{}).Start(context.Background()); err == nil {
		t.Fatal("unbound start succeeded")
	}
	service := backend.NewChatServer(useChatTaskCore(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Init(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("init ignored cancellation: %v", err)
	}
	if err := service.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("start ignored cancellation: %v", err)
	}
	if err := service.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("close ignored cancellation: %v", err)
	}
}
