package Tests

import (
	"path/filepath"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	"github.com/caliniya/caf/utils/id"
)

func TestGeneratedObjectIDsUseTypedUUIDv7(t *testing.T) {
	instance := useChatTaskCore(t)
	instance.Paths.ConfigFile = filepath.Join(instance.Paths.Data, "config", "MainConfig.json")
	if _, err := instance.AddModelConfig("Provider", "https://example.test", "chat-completions", "key", []apiImpl.Model{{ID: "m1"}}); err != nil {
		t.Fatal(err)
	}
	if value := instance.Configs.EnablesConfig.ID; !id.Valid(value, id.Config) {
		t.Fatalf("config ID is not typed UUIDv7: %q", value)
	}
	task, err := instance.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !id.Valid(task.ID, id.Chat) {
		t.Fatalf("chat ID is not typed UUIDv7: %q", task.ID)
	}
	if _, err := instance.GetChat(task.ID); err != nil {
		t.Fatalf("new chat ID rejected on read: %v", err)
	}
	for _, invalid := range []string{strings.TrimPrefix(task.ID, "chat-"), "node-" + strings.TrimPrefix(task.ID, "chat-")} {
		if _, err := instance.GetChat(invalid); err == nil || !strings.Contains(err.Error(), "无效会话 ID") {
			t.Fatalf("legacy or wrong-kind chat ID not rejected: %q / %v", invalid, err)
		}
	}
	node := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "user", Content: "question"})
	if !id.Valid(node.ID, id.Node) {
		t.Fatalf("node ID is not typed UUIDv7: %q", node.ID)
	}
	workspace, err := instance.CreateWorkspace("Workspace", t.TempDir())
	if err != nil || !id.Valid(workspace.ID, id.Workspace) {
		t.Fatalf("workspace did not receive typed UUIDv7: %+v / %v", workspace, err)
	}
	qqCore := useQQAdapterCore(t)
	adapter, err := qqCore.CreateQQAdapter("QQ", "token", "10001")
	if err != nil || !id.Valid(adapter.ID, id.QQAdapter) {
		t.Fatalf("QQ adapter did not receive typed UUIDv7: %+v / %v", adapter, err)
	}
}
