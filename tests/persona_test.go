package Tests

import (
	"encoding/json"
	"strings"
	"testing"

	persona "github.com/caliniya/caf/core/Persona"
)

func TestPersonaSelectionFromSerializedListPersists(t *testing.T) {
	instance := useChatTaskCore(t)
	personas := instance.ListPersonas()
	if len(personas) == 0 {
		t.Fatal("persona selector must have at least one option")
	}
	data, err := json.Marshal(personas)
	if err != nil {
		t.Fatal(err)
	}
	// 使用前端选择器实际读取的 JSON 字段，防止 Go 字段存在但没有传到界面。
	var options []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &options); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for i, option := range options {
		if option.ID == "" || seen[option.ID] {
			t.Fatalf("persona options must have distinct nonempty IDs: %s", data)
		}
		seen[option.ID] = true
		if option.ID != personas[i].ID || option.Name != personas[i].Name {
			t.Fatalf("serialized persona does not match its source: %+v", option)
		}
		t.Run(option.ID, func(t *testing.T) {
			task, err := instance.CreateChat("", "", "", option.ID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Tree.Root.PersonaID != option.ID {
				t.Fatalf("selected %q, created chat with %q", option.ID, task.Tree.Root.PersonaID)
			}
			loaded, err := instance.GetChat(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Tree.Root.PersonaID != option.ID {
				t.Fatalf("selected %q, reloaded chat with %q", option.ID, loaded.Tree.Root.PersonaID)
			}
			if active := persona.Resolve(loaded.Tree.Root.PersonaID); active.System != personas[i].System {
				t.Fatal("reloaded chat resolves to the wrong persona prompt")
			}
		})
	}
}

func TestPersonaPromptContainsRoleContentWithoutSelectionID(t *testing.T) {
	for _, description := range []string{"", "测试角色"} {
		t.Run(description, func(t *testing.T) {
			p := persona.Persona{
				ID:          "selector-only-id",
				Name:        "依依",
				Description: description,
				System:      "你是聊天助手。\n自然地回答包含\"引号\"的问题。",
			}
			prompt, err := p.Prompt()
			if err != nil {
				t.Fatal(err)
			}
			prefix, content, ok := strings.Cut(prompt, "\n")
			if !ok || !strings.Contains(prefix, "角色设定") {
				t.Fatalf("prompt is missing its role instruction: %q", prompt)
			}
			var fields map[string]string
			if err := json.Unmarshal([]byte(content), &fields); err != nil {
				t.Fatal(err)
			}
			if _, exists := fields["id"]; exists {
				t.Fatal("selection ID should not be included in the model prompt")
			}
			if fields["name"] != p.Name || fields["description"] != p.Description || fields["system"] != p.System {
				t.Fatalf("prompt lost role content: %+v", fields)
			}
			if _, exists := fields["description"]; exists != (description != "") {
				t.Fatal("empty descriptions should be omitted from the prompt")
			}
		})
	}
}
