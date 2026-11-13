package Tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	persona "github.com/caliniya/caf/core/Persona"
	platform "github.com/caliniya/caf/platform"
	desktopBackend "github.com/caliniya/caf/platform/backend/desktop"
)

func useChatTaskCore(t *testing.T) *backend.Core {
	t.Helper()
	root := t.TempDir()
	configs := []backend.Config{{
		ID: "provider-1", Name: "Test Provider", ServerAddress: "https://example.test/v1",
		Protocol: "chat-completions", APIKey: "secret-key",
		Model: []apiImpl.Model{{ID: "model-1", Name: "Model 1"}}, EnabledModel: "model-1",
	}}
	instance := &backend.Core{
		Paths: &platform.Paths{Data: root},
		FS:    &desktopBackend.DesktopFileSystem{},
		Configs: &backend.Configs{
			ALLConfig:     &configs,
			EnablesConfig: &configs[0],
		},
	}
	backend.Platform = instance
	return instance
}

func TestChatTaskPersistenceUsesConfigIDAndTreeBranches(t *testing.T) {
	instance := useChatTaskCore(t)
	task, err := instance.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.ConfigID != "provider-1" || task.Model != "model-1" || task.Tree == nil {
		t.Fatalf("unexpected task defaults: %+v", task)
	}
	if task.Tree.Root == nil || task.Tree.Root.Next == nil {
		t.Fatal("new chat tree root branches should be an empty array")
	}
	if task.Tree.Root.PersonaID != persona.DefaultID {
		t.Fatalf("new chat should fall back to default persona, got %q", task.Tree.Root.PersonaID)
	}

	user := task.Tree.Start(apiImpl.ChatMessage{Role: "user", Content: "hello"})
	first := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "first"})
	second := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "second"})
	user.AddNext(first)
	user.AddNext(second)
	taskData, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance.Paths.Data, "chats", task.ID+".json"), taskData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := instance.SetChatBranch(task.ID, user.ID, 0); err != nil {
		t.Fatal(err)
	}
	loaded, err := instance.GetChat(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := loaded.Tree.ToChatRequest(instance.Configs.EnablesConfig.Model[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.Messages) != 3 || messages.Messages[2].Content != "first" {
		t.Fatalf("stored branch was not restored: %+v", messages.Messages)
	}

	data, err := os.ReadFile(filepath.Join(instance.Paths.Data, "chats", task.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-key") || strings.Contains(string(data), "APIKey") {
		t.Fatalf("task JSON must not persist provider credentials: %s", data)
	}
	var persisted chat.ChatTask
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Client != nil {
		t.Fatal("runtime API client should not be serialized")
	}
}

func TestChatTaskConfigReferenceCanBeUpdated(t *testing.T) {
	instance := useChatTaskCore(t)
	task, err := instance.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := instance.UpdateChatModel(task.ID, "missing-provider", "model-1"); err == nil {
		t.Fatal("expected missing config reference to be rejected")
	}
}
