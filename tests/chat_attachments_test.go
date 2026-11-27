package Tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	chat "github.com/caliniya/caf/core/Chat"
	"github.com/caliniya/caf/core/credentials"
)

func imageFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	m.Set(1, 0, color.RGBA{R: 245, G: 39, A: 255})
	var b bytes.Buffer
	checkOK(t, png.Encode(&b, m))
	return b.Bytes(), base64.StdEncoding.EncodeToString(b.Bytes())
}
func imageParts(t *testing.T, r api.ChatCompletionsRequest) []string {
	t.Helper()
	var urls []string
	for _, m := range r.Messages {
		parts, ok := m.Content.([]any)
		if !ok {
			continue
		}
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			if part["type"] == "image_url" {
				if m.Role != "user" {
					t.Error("image attached to a non-user message")
				}
				v, _ := part["image_url"].(map[string]any)
				u, _ := v["url"].(string)
				urls = append(urls, u)
			}
		}
	}
	return urls
}
func TestDesktopImageInputsAndWorkspaceTool(t *testing.T) {
	c, _ := approvalCore(t)
	c.Configs.EnablesConfig.Model[0].InputModalities = []string{"text", "image"}
	raw, encoded := imageFixture(t)
	url := "data:image/png;base64," + encoded
	work := t.TempDir()
	ws, err := c.CreateWorkspace("vision", work)
	checkOK(t, err)
	task, err := c.CreateChat("", "", "", "")
	checkOK(t, err)
	_, err = c.SetChatWorkspace(task.ID, ws.ID)
	checkOK(t, err)
	checkOK(t, os.WriteFile(filepath.Join(work, "screen.png"), raw, 0600))
	uploads := []backend.AttachmentUpload{{Name: "clip.png", Data: encoded}, {Name: "notes.md", Data: base64.StdEncoding.EncodeToString([]byte("来自文本附件"))}}
	var uploadedID string
	responses := approvalModel(t, c,
		func(req api.ChatCompletionsRequest) []api.ToolCall {
			var hasReadImage bool
			for _, tool := range req.Tools {
				hasReadImage = hasReadImage || tool.Function.Name == "ReadImage"
			}
			if !hasReadImage {
				t.Error("visual model was not offered ReadImage")
			}
			u := imageParts(t, req)
			if len(u) != 1 || u[0] != url {
				t.Error("direct input did not contain the real PNG")
			}
			data, _ := json.Marshal(req.Messages)
			if !bytes.Contains(data, []byte("来自文本附件")) {
				t.Error("missing text attachment")
			}
			return nil
		},
		func(req api.ChatCompletionsRequest) []api.ToolCall {
			if len(imageParts(t, req)) != 0 {
				t.Error("replayed old pixels on an unrelated turn")
			}
			return []api.ToolCall{approvalCall("workspace-image", "ReadImage", map[string]any{"source": "screen.png"}), approvalCall("history-image", "ReadImage", map[string]any{"source": uploadedID})}
		},
		func(req api.ChatCompletionsRequest) []api.ToolCall {
			u := imageParts(t, req)
			if len(u) != 2 || u[0] != url || u[1] != url {
				t.Error("tool follow-up missing one or both images")
			}
			n := len(req.Messages)
			if n < 4 || req.Messages[n-3].ToolCallID != "workspace-image" || req.Messages[n-2].ToolCallID != "history-image" || req.Messages[n-1].Role != "user" {
				t.Error("tool result group split by image message")
			}
			return nil
		},
		func(req api.ChatCompletionsRequest) []api.ToolCall {
			if len(imageParts(t, req)) != 1 {
				t.Error("editing the first message lost its image")
			}
			return nil
		},
	)
	checkOK(t, c.SendChatMessageWithAttachments(task.ID, "", "high", uploads))
	if responses.Load() != 1 {
		t.Fatal("direct images triggered a separate request")
	}
	s, err := c.GetChat(task.ID)
	checkOK(t, err)
	path, err := s.Tree.CurrentPath()
	checkOK(t, err)
	if len(path) != 2 || len(path[0].Child[0].Attachments) != 2 || len(path[1].InputImages) != 1 {
		t.Fatal("did not persist image delivery and attachments")
	}
	first := path[0]
	uploadedID = first.Child[0].Attachments[0].ID
	got, err := c.GetChatImage(task.ID, uploadedID)
	checkOK(t, err)
	if got != url {
		t.Fatal("preview differs from sent bytes")
	}
	// Replaying a previous image must re-check the model, before saving a new branch.
	c.Configs.EnablesConfig.Model[0].InputModalities = []string{"text"}
	if err := c.EditChatMessage(task.ID, first.ID, "改写", "high"); err == nil || !strings.Contains(err.Error(), "不支持图片") {
		t.Fatal("editing replayed an image to a text model", err)
	}
	if err := c.RegenerateChatMessage(task.ID, path[1].ID, "high"); err == nil || !strings.Contains(err.Error(), "不支持图片") {
		t.Fatal("regenerating replayed an image to a text model", err)
	}
	if responses.Load() != 1 {
		t.Fatal("blocked image replay reached provider")
	}
	c.Configs.EnablesConfig.Model[0].InputModalities = []string{"text", "image"}
	checkOK(t, c.SendChatMessage(task.ID, "现在请看工作区图片和上一张图", "high"))
	if responses.Load() != 3 {
		t.Fatal("two tool reads should use one follow-up request")
	}
	s, err = c.GetChat(task.ID)
	checkOK(t, err)
	path, err = s.Tree.CurrentPath()
	checkOK(t, err)
	calls := path[len(path)-1].Child[0].ToolCalls
	if len(calls) != 2 || calls[0].Image == nil || !calls[0].ImageSent || !calls[1].ImageSent {
		t.Fatal("did not persist successful ReadImage receipts")
	}
	workID := calls[0].Image.ID
	checkOK(t, os.WriteFile(filepath.Join(work, "screen.png"), []byte("changed"), 0600))
	got, err = c.GetChatImage(task.ID, workID)
	checkOK(t, err)
	if got != url {
		t.Fatal("preview reopened the mutable workspace file")
	}
	other, err := c.CreateChat("", "", "", "")
	checkOK(t, err)
	if _, err = c.GetChatImage(other.ID, uploadedID); err == nil {
		t.Fatal("allowed preview from a different chat")
	}
	checkOK(t, c.EditChatMessage(task.ID, first.ID, "编辑后的文字", "high"))
	if _, err = c.GetChatImage(task.ID, workID); err == nil {
		t.Fatal("allowed an image from a sibling branch")
	}
	_, err = c.GetChatImage(task.ID, uploadedID)
	checkOK(t, err)
	index, err := os.ReadFile(filepath.Join(c.Paths.Data, "chats", task.ID+".json"))
	checkOK(t, err)
	if bytes.Contains(index, []byte(encoded)) {
		t.Fatal("stored Base64 in chat JSON")
	}
	checkOK(t, c.DeleteChat(task.ID))
	if _, err = os.Stat(filepath.Join(c.Paths.Data, "chats", "attachments", task.ID)); !os.IsNotExist(err) {
		t.Fatal("deleted chat retained its images", err)
	}
}
func TestChatImageFailuresAndModelCapability(t *testing.T) {
	c, _ := approvalCore(t)
	s, e := c.CreateChat("", "", "", "")
	checkOK(t, e)
	_, encoded := imageFixture(t)
	var hits atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "image not supported", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer provider.Close()
	cfg := c.Configs.EnablesConfig
	cfg.ServerAddress = provider.URL
	cfg.Model[0].InputModalities = []string{"text"}
	send := func(name, b64 string) error {
		return c.SendChatMessageWithAttachments(s.ID, "", "high", []backend.AttachmentUpload{{Name: name, Data: b64}})
	}
	if err := send("a.png", encoded); err == nil || !strings.Contains(err.Error(), "不支持图片") {
		t.Fatal("text-only model accepted an image")
	}
	cfg.Model[0].InputModalities = nil
	if err := send("a.png", encoded); err == nil || !strings.Contains(err.Error(), "尚未确认") {
		t.Fatal("unconfirmed model accepted an image", err)
	}
	unchanged, err := c.GetChat(s.ID)
	checkOK(t, err)
	if path, err := unchanged.Tree.CurrentPath(); err != nil || len(path) != 0 || hits.Load() != 0 {
		t.Fatal("blocked images changed the chat or contacted the provider")
	}
	cfg.Model[0].InputModalities = []string{"text", "image"}
	for _, f := range [][2]string{{"broken.png", base64.StdEncoding.EncodeToString([]byte("not an image"))}, {"../bad.png", encoded}, {"bad.png", encoded[:len(encoded)/2]}, {"doc.pdf", encoded}} {
		if send(f[0], f[1]) == nil {
			t.Fatal("accepted an invalid file", f[0])
		}
	}
	if hits.Load() != 0 {
		t.Fatal("local validation charged an API request")
	}
	if send("a.png", encoded) == nil || hits.Load() != 1 {
		t.Fatal("provider rejection did not propagate")
	}
	result, e := c.GetChat(s.ID)
	checkOK(t, e)
	p, e := result.Tree.CurrentPath()
	checkOK(t, e)
	if len(p[1].InputImages) != 0 {
		t.Fatal("claimed that a rejected image was read")
	}
	if send("only-done.png", encoded) == nil || hits.Load() != 2 {
		t.Fatal("a stream terminator alone should not acknowledge the image")
	}
	result, e = c.GetChat(s.ID)
	checkOK(t, e)
	p, e = result.Tree.CurrentPath()
	checkOK(t, e)
	if len(p[len(p)-1].InputImages) != 0 {
		t.Fatal("claimed that an empty stream read an image")
	}
}
func TestChatAttachmentVaultAndExport(t *testing.T) {
	files := &profilePicker{dest: t.TempDir()}
	base := t.TempDir()
	accounts := openProfiles(t, base, files, &testDeviceKeys{})
	checkOK(t, accounts.CreateVault("owner-vision"))
	ticket := profileStart(t, accounts)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { qqSSE(w, "received") }))
	defer server.Close()
	profileCall(t, accounts, ticket, "AddModelConfig", "vision", server.URL, "", "", []api.Model{{ID: "vision", InputModalities: []string{"image", "text"}}})
	var s chat.ChatTask
	checkOK(t, json.Unmarshal([]byte(profileCall(t, accounts, ticket, "CreateChat", "", "", "", "")), &s))
	raw, b64 := imageFixture(t)
	profileCall(t, accounts, ticket, "SendChatMessageWithAttachments", s.ID, "", "high", []backend.AttachmentUpload{{Name: "image.png", Data: b64}})
	checkOK(t, json.Unmarshal([]byte(profileCall(t, accounts, ticket, "GetChat", s.ID)), &s))
	p, _ := s.Tree.CurrentPath()
	a := p[0].Child[0].Attachments[0]
	rel := "chats/attachments/" + s.ID + "/" + a.ID + ".png"
	disk, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
	checkOK(t, err)
	if !credentials.IsEncryptedChat(disk) || bytes.Contains(disk, raw) {
		t.Fatal("image was saved unencrypted")
	}
	archive, err := accounts.ExportUserData(ticket, "owner-vision", false)
	checkOK(t, err)
	if zipFiles(t, archive)[rel] != string(raw) {
		t.Fatal("export did not contain the original PNG")
	}
	_, err = accounts.CreateInternalUser(ticket, "guest-vision", nil)
	checkOK(t, err)
	checkOK(t, accounts.Logout(ticket))
	checkOK(t, accounts.UnlockVault("guest-vision"))
	guest := profileStart(t, accounts)
	args, _ := json.Marshal([]string{s.ID, a.ID})
	if _, err = accounts.Call(guest, "GetChatImage", string(args)); err == nil {
		t.Fatal("internal user read the owner's image")
	}
	checkOK(t, accounts.Logout(guest))
	checkOK(t, accounts.UnlockVault("owner-vision"))
	ticket = profileStart(t, accounts)
	if got := profileCall(t, accounts, ticket, "GetChatImage", s.ID, a.ID); !strings.Contains(got, b64) {
		t.Fatal("owner preview did not survive login")
	}
}

func TestStopDesktopGenerationPreservesToolsAndNextReply(t *testing.T) {
	c, _ := approvalCore(t)
	workspace := t.TempDir()
	w, err := c.CreateWorkspace("stop", workspace)
	checkOK(t, err)
	s, err := c.CreateChat("", "", "", "")
	checkOK(t, err)
	_, err = c.SetChatWorkspace(s.ID, w.ID)
	checkOK(t, err)
	calls := approvalModel(t, c, func(_ api.ChatCompletionsRequest) []api.ToolCall {
		return []api.ToolCall{
			approvalCall("changed", "WriteFile", map[string]any{"path": []string{"kept.txt"}, "content": "kept"}),
			approvalCall("waiting", "AskUser", map[string]any{"question": "继续吗？"}),
			approvalCall("skipped", "WriteFile", map[string]any{"path": []string{"skipped.txt"}, "content": "skip"}),
		}
	})
	node, waiting := make(chan string, 1), make(chan struct{}, 1)
	previous := backend.Emit
	t.Cleanup(func() { backend.Emit = previous })
	backend.Emit = func(event string, data ...any) {
		if event != backend.EventChatGeneration && event != backend.EventChatTool {
			return
		}
		var e map[string]string
		if json.Unmarshal([]byte(data[0].(string)), &e) != nil {
			return
		}
		if event == backend.EventChatGeneration {
			select {
			case node <- e["nodeId"]:
			default:
			}
		}
		if e["callId"] == "waiting" && e["requestId"] != "" {
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
	}
	done := make(chan error, 1)
	go func() { done <- c.SendChatMessage(s.ID, "先写入，再询问", "high") }()
	var current string
	select {
	case current = <-node:
	case e := <-done:
		t.Fatal("ended before start", e)
	case <-time.After(3 * time.Second):
		t.Fatal("no start event")
	}
	defer c.StopChatGeneration(s.ID, current)
	select {
	case <-waiting:
	case e := <-done:
		t.Fatal("ended before question", e)
	case <-time.After(3 * time.Second):
		t.Fatal("question did not start")
	}
	checkOK(t, c.StopChatGeneration(s.ID, "stale-reply"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	checkOK(t, c.StopChatGeneration(s.ID, current))
	select {
	case err := <-done:
		checkOK(t, err)
	case <-ctx.Done():
		t.Fatal("stop did not release waiting tool")
	}
	loaded, e := c.GetChat(s.ID)
	checkOK(t, e)
	path, e := loaded.Tree.CurrentPath()
	checkOK(t, e)
	reply := path[len(path)-1]
	if !reply.Stopped || len(reply.ToolOperations) != 1 {
		t.Fatal("lost stop marker or previously completed file operation")
	}
	results := map[string]api.ChatMessage{}
	for _, m := range reply.Child {
		if m.Role == "tool" {
			results[m.ToolCallID] = m
		}
	}
	if len(results) != 3 || results["skipped"].ToolError == "" || results["changed"].ToolError != "" {
		t.Fatal("incomplete tool protocol after stop", results)
	}
	b, e := os.ReadFile(filepath.Join(workspace, "kept.txt"))
	checkOK(t, e)
	if string(b) != "kept" {
		t.Fatal("stop undid an already finished modification")
	}
	if _, e = os.Stat(filepath.Join(workspace, "skipped.txt")); !os.IsNotExist(e) {
		t.Fatal("tool executed after stop")
	}
	checkOK(t, c.SendChatMessage(s.ID, "继续", "high"))
	if calls.Load() != 2 {
		t.Fatal("could not continue after stopping")
	}
}

// A stop before the first visible token must leave a retryable UI node without
// an invalid empty assistant in the next provider request.
func TestStopBeforeFirstAnswerCanContinueAndRegenerate(t *testing.T) {
	for _, mode := range []string{"before-stream", "reasoning-only"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := approvalCore(t)
			s, err := c.CreateChat("", "", "", "")
			checkOK(t, err)
			var firstID string
			target := make(chan string, 1)
			previous := backend.Emit
			t.Cleanup(func() { backend.Emit = previous })
			backend.Emit = func(event string, data ...any) {
				if event != backend.EventChatGeneration && event != backend.EventChatDelta {
					return
				}
				var e struct{ NodeID, Reasoning string }
				if json.Unmarshal([]byte(data[0].(string)), &e) != nil {
					return
				}
				if event == backend.EventChatGeneration && firstID == "" {
					firstID = e.NodeID
					target <- firstID
				}
				if e.NodeID == firstID && e.Reasoning != "" {
					checkOK(t, c.StopChatGeneration(s.ID, firstID))
				}
			}
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request api.ChatCompletionsRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if hits.Add(1) == 1 {
					node := <-target
					if mode == "before-stream" {
						if err := c.StopChatGeneration(s.ID, node); err != nil {
							t.Error(err)
						}
					} else {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"思考中\"}}]}\n\n")
						w.(http.Flusher).Flush()
					}
					select {
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
						t.Error("provider request was not cancelled")
					}
					return
				}
				for _, m := range request.Messages {
					if m.Role == "assistant" && (m.Content == nil || m.Content == "") && len(m.ToolCalls) == 0 {
						http.Error(w, "empty assistant", http.StatusBadRequest)
						return
					}
				}
				qqSSE(w, "新的回复")
			}))
			defer server.Close()
			c.Configs.EnablesConfig.ServerAddress = server.URL
			checkOK(t, c.SendChatMessage(s.ID, "第一条消息", "high"))
			s, err = c.GetChat(s.ID)
			checkOK(t, err)
			node := s.Tree.FindNode(firstID)
			if node == nil || !node.Stopped {
				t.Fatal("stopped reply was lost")
			}
			if mode == "before-stream" && len(node.Child) != 0 {
				t.Fatal("created an empty assistant before any output")
			}
			if mode == "reasoning-only" && (len(node.Child) != 1 || node.Child[0].Reasoning != "思考中") {
				t.Fatal("lost partial reasoning")
			}
			checkOK(t, c.SendChatMessage(s.ID, "继续", "high"))
			checkOK(t, c.RegenerateChatMessage(s.ID, firstID, "high"))
			if hits.Load() != 3 {
				t.Fatal("unexpected follow-up count", hits.Load())
			}
		})
	}
}

func TestImageToolDeniedAndDirectInputIndependentOfKit(t *testing.T) {
	c, _ := approvalCore(t)
	c.Configs.EnablesConfig.Model[0].InputModalities = []string{"text", "image"}
	s, e := c.CreateChat("", "", "", "")
	checkOK(t, e)
	_, b64 := imageFixture(t)
	_, e = c.SetChatToolConfig(s.ID, `{"sets":{"主工具集":{"kits":{"扩展交互工具":false}}}}`)
	checkOK(t, e)
	requests := approvalModel(t, c, func(r api.ChatCompletionsRequest) []api.ToolCall {
		if len(imageParts(t, r)) != 1 {
			t.Error("explicit user-provided image was blocked by tool settings")
		}
		for _, td := range r.Tools {
			if td.Function.Name == "ReadImage" {
				t.Error("disabled ReadImage still advertised")
			}
		}
		return []api.ToolCall{approvalCall("denied", "ReadImage", map[string]any{"source": "screen.png"})}
	}, func(r api.ChatCompletionsRequest) []api.ToolCall {
		parts := imageParts(t, r)
		if len(parts) != 1 {
			t.Error("original current-turn input should still be visible during tool continuation")
		}
		last := r.Messages[len(r.Messages)-1]
		if last.Role != "tool" || last.ToolCallID != "denied" {
			t.Error("disabled tool delivered a separate image")
		}
		return nil
	})
	checkOK(t, c.SendChatMessageWithAttachments(s.ID, "", "high", []backend.AttachmentUpload{{Name: "direct.png", Data: b64}}))
	if requests.Load() != 2 {
		t.Fatal("extra read request")
	}
	loaded, e := c.GetChat(s.ID)
	checkOK(t, e)
	path, _ := loaded.Tree.CurrentPath()
	reply := path[1]
	if len(reply.InputImages) != 1 || reply.Child[0].ToolCalls[0].Image != nil || reply.Child[0].ToolCalls[0].ImageSent {
		t.Fatal("disabled tool recorded a read or direct image was acknowledged twice")
	}
}
