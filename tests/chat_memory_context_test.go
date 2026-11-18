package Tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	memory "github.com/caliniya/caf/Memory"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
)

func TestChatMemoryContextExcludesDisabledLibrariesAndLocksSettingsDuringGeneration(t *testing.T) {
	useToolRegistry(t)
	core := useChatTaskCore(t)
	settings, err := core.GetChatMemorySettings("")
	if err != nil {
		t.Fatal(err)
	}
	server, _ := core.Servers.Get[*memory.MemoryServer](memory.MemoryServerName)
	globalID := settings.Libraries[0].ID
	disabled, err := server.CreateRecollection(context.Background(), "Disabled", "", memory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	entryID, err := server.AddMemory(context.Background(), globalID, "visible-memory-marker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.AddMemory(context.Background(), disabled.ID, "disabled-memory-marker"); err != nil {
		t.Fatal(err)
	}
	task, err := core.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatMemoryLibraries(task.ID, []string{globalID}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request apiImpl.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var contextText strings.Builder
		for _, message := range request.Messages {
			if message.Role == "system" {
				fmt.Fprint(&contextText, message.Content)
			}
		}
		if strings.Contains(contextText.String(), "disabled-memory-marker") {
			t.Error("disabled library leaked into model context")
		}
		if strings.Contains(contextText.String(), "visible-memory-marker") != (calls == 1) {
			t.Error("enabled memory did not follow the saved selection")
		}
		if _, err := core.SetChatMemoryLibraries(task.ID, []string{disabled.ID}); err == nil {
			t.Error("memory settings changed during generation")
		}
		if _, err := core.CreateMemoryLibrary(task.ID, "", "DuringGeneration", memory.ScopeGlobal); err == nil {
			t.Error("memory created during generation")
		}
		if err := core.RenameMemoryLibrary(task.ID, "", disabled.ID, "RenamedDuringGeneration"); err == nil {
			t.Error("memory renamed during generation")
		}
		if err := core.DeleteMemoryLibrary(task.ID, "", disabled.ID); err == nil {
			t.Error("memory deleted during generation")
		}
		if err := core.SetMemoryLibraryScope(task.ID, "", disabled.ID, memory.ScopeGlobal); err == nil {
			t.Error("memory scope changed during generation")
		}
		input := backend.MemoryEntryInput{Name: "名称", Data: "正文", Date: "2026-11-18T08:00:00Z"}
		if _, err := core.CreateMemoryEntry(task.ID, "", globalID, input); err == nil {
			t.Error("entry created during generation")
		}
		if _, err := core.UpdateMemoryEntry(task.ID, "", globalID, entryID, input); err == nil {
			t.Error("entry edited during generation")
		}
		if _, err := core.DeleteMemoryEntry(task.ID, "", globalID, entryID); err == nil {
			t.Error("entry deleted during generation")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"reply\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	core.Configs.EnablesConfig.ServerAddress = model.URL
	if err := core.SendChatMessage(task.ID, "question", "high"); err != nil {
		t.Fatal(err)
	}
	if _, err := core.SetChatMemoryLibraries(task.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := core.SendChatMessage(task.ID, "continue without memory", "high"); err != nil {
		t.Fatal(err)
	}
}
