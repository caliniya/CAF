package Backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	tool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	tree "github.com/caliniya/caf/core/Chat/ChatStruct"
	"github.com/caliniya/caf/utils/id"
)

type imageDelivery struct {
	ref    api.Attachment
	pixels api.InputImage
	callID string
}

// Keep directly attached images available throughout this assistant turn, including any
// tool continuation. Future user turns retain references, not the pixels.
type chatVision struct {
	core              *Core
	directory, chatID string
	model             api.Model
	known             map[string]api.Attachment
	direct            []imageDelivery
	pending           []imageDelivery
}

func newChatVision(c *Core, directory string, s *chat.ChatTask, reply *tree.ChatNode, m api.Model) (*chatVision, error) {
	refs, err := branchImages(s)
	if err != nil {
		return nil, err
	}
	v := &chatVision{core: c, directory: directory, chatID: s.ID, model: m, known: refs}
	parent, _ := s.Tree.ParentOf(reply.ID)
	if parent != nil {
		for _, msg := range parent.Child {
			if msg.Role != "user" {
				continue
			}
			for _, a := range msg.Attachments {
				if !a.IsImage() {
					continue
				}
				if !m.AllowsImages() {
					return nil, errors.New("当前模型不支持图片输入，请切换视觉模型后重试")
				}
				img, err := c.loadImage(directory, s.ID, a)
				if err != nil {
					return nil, fmt.Errorf("读取附件 %s：%w", a.Name, err)
				}
				v.pending = append(v.pending, imageDelivery{a, img, ""})
			}
		}
	}
	v.direct = slices.Clone(v.pending)
	return v, nil
}
func (v *chatVision) read(ctx context.Context, tc *tool.ToolContext, source string) (string, error) {
	if !v.model.AllowsImages() {
		return "", errors.New("当前模型不支持图片输入")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(v.pending) >= maxAttachments {
		return "", errors.New("一次请求最多读取 4 张图片")
	}
	a, existing := v.known[source]
	var img api.InputImage
	var err error
	if existing {
		img, err = v.core.loadImage(v.directory, v.chatID, a)
	} else {
		if id.Valid(source, id.Attachment) {
			return "", errors.New("当前对话分支没有这张图片")
		}
		if tc.WorkSpace == "" || !filepath.IsLocal(source) {
			return "", errors.New("请选择当前工作区内的相对路径或当前分支的图片附件编号")
		}
		if _, err = tc.ResolvePath(source); err != nil {
			return "", err
		}
		var data []byte
		data, err = boundedRead(tc.WorkSpace, source, maxFileBytes)
		if err == nil {
			img, err = validatedImage(filepath.Base(source), data)
		}
		if err == nil {
			a = imageRef(img)
		}
	}
	if err != nil {
		return "", err
	}
	total := len(img.Data)
	for _, p := range v.pending {
		total += len(p.pixels.Data)
	}
	if total > maxAttachmentBytes {
		return "", errors.New("本轮图片总大小超过 12 MiB")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if !existing {
		if err = v.core.saveImage(v.directory, v.chatID, a, img.Data); err != nil {
			return "", err
		}
		v.known[a.ID] = a
	}
	v.pending = append(v.pending, imageDelivery{a, img, tc.CallID})
	return fmt.Sprintf("已载入图片 %q（%s），像素随本轮工具结果提供。", a.Name, a.ID), nil
}

// Chat Completions tool responses are text. Keep all tool_call_id/result pairs together
// and attach their images in one user message, in the normal follow-up request.
func (v *chatVision) messages(history []api.ChatMessage) []api.ChatMessage {
	if len(v.pending) == 0 && len(v.direct) == 0 {
		return history
	}
	messages := slices.Clone(history)
	attach := func(at int, items []imageDelivery) {
		if at < 0 {
			at = len(messages)
			messages = append(messages, api.ChatMessage{Role: "user", Content: "本轮提供的图片："})
		}
		messages[at].Images = make([]api.InputImage, 0, len(items))
		messages[at].Tokens = 0
		for _, p := range items {
			messages[at].Images = append(messages[at].Images, p.pixels)
		}
	}
	if len(v.direct) > 0 {
		at := -1
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" && slices.ContainsFunc(messages[i].Attachments, func(a api.Attachment) bool { return a.ID == v.direct[0].ref.ID }) {
				at = i
				break
			}
		}
		attach(at, v.direct)
	}
	if len(v.pending) > 0 && v.pending[0].callID != "" {
		attach(-1, v.pending)
	}
	return messages
}
func (v *chatVision) received(reply *tree.ChatNode) {
	for _, p := range v.pending {
		if p.callID == "" {
			reply.InputImages = append(reply.InputImages, p.ref)
		} else {
			for _, msg := range reply.Child {
				for i := range msg.ToolCalls {
					if msg.ToolCalls[i].ID == p.callID {
						msg.ToolCalls[i].ImageSent = true
					}
				}
			}
		}
		EmitJSON(EventChatTool, map[string]any{"chatId": v.chatID, "nodeId": reply.ID, "toolId": tool.ReadImageID, "callId": p.callID, "event": "image", "name": "读取图片", "image": p.ref})
	}
	v.pending = nil
}
