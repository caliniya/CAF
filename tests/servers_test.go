package Tests

import (
	"context"
	"fmt"
	"sync"
	"testing"

	backend "github.com/caliniya/caf/core/Backend"
)

func TestServersRejectInvalidRegistrationAndPreserveInstances(t *testing.T) {
	var registry backend.Servers
	var nilService *otherTestServer
	for _, invalid := range []struct {
		name    string
		service backend.Server
	}{
		{"", &otherTestServer{}}, {" ", &otherTestServer{}}, {"nil", nil}, {"typed-nil", nilService},
	} {
		if err := registry.Register(invalid.name, invalid.service); err == nil {
			t.Fatalf("invalid service registration accepted: %q", invalid.name)
		}
	}
	service := &otherTestServer{}
	if err := registry.Register("custom", service); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("custom", &otherTestServer{}); err == nil {
		t.Fatal("duplicate registration replaced a running service")
	}
	got, err := registry.Get[*otherTestServer]("custom")
	if err != nil || got != service {
		t.Fatalf("registration did not preserve identity: %p / %v", got, err)
	}
	if got, err := registry.Get[*backend.QQServer]("custom"); err == nil || got != nil {
		t.Fatalf("incorrect service type was accepted: %p / %v", got, err)
	}
}

func TestServersConcurrentRegisterAndGet(t *testing.T) {
	var registry backend.Servers
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("service-%d", i)
			service := &otherTestServer{}
			if err := registry.Register(name, service); err != nil {
				t.Error(err)
				return
			}
			for range 20 {
				got, err := registry.Get[*otherTestServer](name)
				if err != nil || got != service {
					t.Errorf("concurrent read lost service: %q / %v", name, err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestCoreInitServersPreservesCustomService(t *testing.T) {
	core := &backend.Core{}
	custom := backend.NewQQServer(core)
	custom.ListenHost = "127.0.0.1"
	if err := core.Servers.Register(backend.QQServerName, custom); err != nil {
		t.Fatal(err)
	}
	core.InitServers()
	chat, err := core.Servers.Get[*backend.ChatServer](backend.ChatServerName)
	if err != nil {
		t.Fatal(err)
	}
	core.InitServers()
	got, err := core.Servers.Get[*backend.QQServer](backend.QQServerName)
	if err != nil || got != custom || got.ListenHost != "127.0.0.1" {
		t.Fatalf("default registration replaced custom service: %+v / %v", got, err)
	}
	chatAgain, err := core.Servers.Get[*backend.ChatServer](backend.ChatServerName)
	if err != nil || chatAgain != chat {
		t.Fatal("repeated initialization replaced the chat service")
	}
	if err := chat.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := backend.NewChatServer(core).Start(context.Background()); err == nil {
		t.Fatal("an unregistered chat instance used the registered instance's state")
	}
}

func TestCoreServiceAPIsReportMissingAndMismatchedServices(t *testing.T) {
	core := &backend.Core{}
	if _, err := core.ListChats(); err == nil {
		t.Fatal("chat API accepted a missing service")
	}
	if _, err := core.ListQQAdapters(); err == nil {
		t.Fatal("QQ API accepted a missing service")
	}
	if err := core.SubmitUserAnswer("chat", "request", "answer"); err == nil {
		t.Fatal("interaction API accepted a missing service")
	}
	if _, err := core.ListWorkspaces(); err == nil {
		t.Fatal("workspace API accepted a missing chat service")
	}
	if err := core.Servers.Register(backend.ChatServerName, &otherTestServer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.ListChats(); err == nil {
		t.Fatal("chat API accepted an incorrect registered service type")
	}
}
