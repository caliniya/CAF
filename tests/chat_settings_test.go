package Tests

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	texts "github.com/caliniya/caf/utils/text"
)

func TestChatDefaultsRoundTripAndMemorySnapshot(t *testing.T) {
	c, _ := approvalCore(t)
	c.Paths.ConfigFile = filepath.Join(c.Paths.Data, "MainConfig.json")
	existing, err := c.CreateChat("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	library, err := c.GetChatMemorySettings("")
	if err != nil {
		t.Fatal(err)
	}
	id := library.Libraries[0].ID
	value, err := c.GetChatDefaults()
	if err != nil || !value.Compaction.Enabled || value.Compaction.Automatic || value.Compaction.WindowPercent != 70 || value.Compaction.TokenThreshold != 500000 {
		t.Fatalf("unexpected built-in defaults: %+v, %v", value, err)
	}
	value.MemoryLibraryIDs = []string{id, "deleted", id}
	value.Tools = &agenttool.ChatToolConfig{Permission: agenttool.PermissionReadOnly}
	catalog := c.ListToolSets()
	set := catalog[0]
	off := false
	value.Tools.SetKit(set.ID, set.Kits[0].ID, &off)
	value.Compaction.WindowPercent = 65
	value.Compaction.TokenThreshold = 60000
	if _, err := c.SetChatDefaults(value); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	saved, err := c.GetChatDefaults()
	if err != nil {
		t.Fatal(err)
	}
	toggled, explicit := saved.Tools.Kit(set.ID, set.Kits[0].ID)
	if !explicit || toggled || saved.Tools.Permission != agenttool.PermissionReadOnly || !slices.Equal(saved.MemoryLibraryIDs, []string{id}) || saved.Compaction != value.Compaction {
		t.Fatalf("defaults failed to round trip: %+v", saved)
	}
	draft, err := c.GetWorkspaceMemorySettings("")
	if err != nil || !slices.Equal(draft.EnabledIDs, []string{id}) {
		t.Fatalf("draft: %+v %v", draft, err)
	}
	newer, err := c.CreateChat("", "", "", "")
	if err != nil || !slices.Equal(newer.MemoryLibraryIDs, []string{id}) {
		t.Fatalf("new chat: %+v, %v", newer, err)
	}
	old, err := c.GetChat(existing.ID)
	if err != nil || len(old.MemoryLibraryIDs) != 0 {
		t.Fatal("existing memory selection changed", err)
	}
	mode := &chat.CompactionMode{Automatic: true}
	if _, err := c.SetChatCompactionMode(newer.ID, mode); err != nil {
		t.Fatal(err)
	}
	newer, err = c.GetChat(newer.ID)
	if err != nil || newer.CompactionMode == nil || newer.CompactionMode.Enabled || newer.CompactionMode.Automatic {
		t.Fatal("disabled override was not preserved", err)
	}
	newer, err = c.SetChatCompactionMode(newer.ID, nil)
	if err != nil || newer.CompactionMode != nil {
		t.Fatal("could not restore inheritance", err)
	}
	for _, field := range []string{"percent", "tokens", "grant", "kit"} {
		invalid, _ := c.GetChatDefaults()
		switch field {
		case "percent":
			invalid.Compaction.WindowPercent = 101
		case "tokens":
			invalid.Compaction.TokenThreshold = 0
		case "grant":
			invalid.Tools.AllowedTools = map[string]agenttool.ToolGrant{"DeleteFile": {Scope: agenttool.GrantTool}}
		case "kit":
			invalid.Tools.SetKit(set.ID, "unknown", &off)
		}
		if _, err := c.SetChatDefaults(invalid); err == nil {
			t.Fatalf("accepted invalid %s setting", field)
		}
	}
	if _, err := c.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	still, _ := c.GetChatDefaults()
	if still.Compaction != saved.Compaction {
		t.Fatal("invalid write altered settings")
	}
}

func TestCompactionReminderTwoThresholds(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		window, percent, absolute int
		accept                    bool
	}{
		{"absolute first then accept", 120000, 50, 30000, true},
		{"percentage first then skip", 120000, 25, 60000, false},
		{"same thresholds", 120000, 25, 30000, false},
		{"unknown window", 0, 70, 30000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, task, _, _ := useCompactionChat(t)
			m := &c.Configs.EnablesConfig.Model[0]
			m.MaxContext, m.MaxOutputTokens = tc.window, 2048
			c.Configs.DefaultCompaction = &backend.CompactionDefaults{CompactionMode: chat.CompactionMode{Enabled: true}, WindowPercent: tc.percent, TokenThreshold: tc.absolute}
			var normal, summaries atomic.Int32
			compactionHTTPModel(t, c, func(w http.ResponseWriter, r *http.Request, req api.ChatCompletionsRequest) {
				switch req.Messages[0].Content {
				case texts.CompactionSummarize:
					summaries.Add(1)
					qqSSE(w, compactionCandidateText("reviewed summary"))
				case texts.CompactionReview:
					qqSSE(w, `{"passed":true,"issues":[]}`)
				default:
					normal.Add(1)
					qqSSE(w, "continued")
				}
			})
			var prompts []backend.CompactionPrompt
			old := backend.Emit
			defer func() { backend.Emit = old }()
			backend.Emit = func(event string, data ...any) {
				if event != backend.EventChatCompaction {
					return
				}
				var p backend.CompactionPrompt
				if err := json.Unmarshal([]byte(data[0].(string)), &p); err != nil {
					t.Fatal(err)
				}
				if p.Used == 0 {
					return
				}
				prompts = append(prompts, p)
				seen, err := c.GetPendingCompaction(task.ID)
				if err != nil || seen == nil || *seen != p {
					t.Fatal("pending prompt cannot be recovered", err)
				}
				if c.SubmitCompactionDecision("another-chat", p.RequestID, true) == nil || c.SubmitCompactionDecision(task.ID, "stale", true) == nil {
					t.Fatal("accepted unrelated decision")
				}
				if err := c.SubmitCompactionDecision(task.ID, p.RequestID, len(prompts) == 2 && tc.accept); err != nil {
					t.Fatal(err)
				}
				if c.SubmitCompactionDecision(task.ID, p.RequestID, true) == nil {
					t.Fatal("accepted duplicate answer")
				}
			}
			send := func(text string) {
				t.Helper()
				if err := c.SendChatMessage(task.ID, text, "high"); err != nil {
					t.Fatal(err)
				}
			}
			send("first")
			send("same range")
			if len(prompts) != 1 || prompts[0].Threshold != 30000 || summaries.Load() != 0 || normal.Load() != 2 {
				t.Fatalf("unexpected first range: %+v", prompts)
			}
			// A large new user message crosses the other threshold. Each send reloads the persisted branch.
			send(strings.Repeat("a", 84000))
			send("still same range")
			if tc.window == 0 || tc.percent*tc.window/100 == tc.absolute {
				if len(prompts) != 1 || prompts[0].NextThreshold != 0 {
					t.Fatalf("should only prompt once: %+v", prompts)
				}
			} else if len(prompts) != 2 || prompts[0].NextThreshold != 60000 || prompts[1].Threshold != 60000 || prompts[1].NextThreshold != 0 {
				t.Fatalf("expected higher threshold only once: %+v", prompts)
			}
			if (summaries.Load() == 1) != tc.accept || normal.Load() != 4 {
				t.Fatalf("unexpected model calls: summaries=%d normal=%d", summaries.Load(), normal.Load())
			}
			if tc.name == "percentage first then skip" {
				// The other continuation from the same last user has not declined any reminders.
				loaded, err := c.GetChat(task.ID)
				if err != nil {
					t.Fatal(err)
				}
				path, err := loaded.Tree.CurrentPath()
				if err != nil {
					t.Fatal(err)
				}
				if err := c.RegenerateChatMessage(task.ID, path[11].ID, "high"); err != nil {
					t.Fatal(err)
				}
				if len(prompts) != 3 || prompts[2].Threshold != 30000 || normal.Load() != 5 {
					t.Fatal("a sibling branch inherited the other branch's dismissal")
				}
			}
			if pending, _ := c.GetPendingCompaction(task.ID); pending != nil {
				t.Fatal("stale decision after reply")
			}
		})
	}
}

func TestCompactionDisabledOverrideAndOneShot(t *testing.T) {
	c, task, _, _ := useCompactionChat(t)
	c.Configs.DefaultCompaction = &backend.CompactionDefaults{CompactionMode: chat.CompactionMode{Enabled: true, Automatic: true}, WindowPercent: 70, TokenThreshold: 20000}
	if _, err := c.SetChatCompactionMode(task.ID, &chat.CompactionMode{}); err != nil {
		t.Fatal(err)
	}
	var summaries atomic.Int32
	compactionHTTPModel(t, c, func(w http.ResponseWriter, _ *http.Request, req api.ChatCompletionsRequest) {
		switch req.Messages[0].Content {
		case texts.CompactionSummarize:
			summaries.Add(1)
			qqSSE(w, compactionCandidateText("manual only"))
		case texts.CompactionReview:
			qqSSE(w, `{"passed":true,"issues":[]}`)
		default:
			qqSSE(w, "unchanged")
		}
	})
	if err := c.SendChatMessage(task.ID, "normal reply", "high"); err != nil || summaries.Load() != 0 {
		t.Fatal("disabled mode ran compaction", err)
	}
	saved, err := c.CompactChat(task.ID)
	if err != nil || summaries.Load() != 1 || saved.CompactionMode == nil || saved.CompactionMode.Enabled {
		t.Fatal("one-shot compaction changed mode", err)
	}
}
