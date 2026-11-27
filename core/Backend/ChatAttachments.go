package Backend

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	chat "github.com/caliniya/caf/core/Chat"
	"github.com/caliniya/caf/utils/id"
)

const (
	maxFileBytes       = 8 << 20
	maxAttachmentBytes = 12 << 20
	maxTextBytes       = 256 << 10
	maxAttachments     = 4
)

// Data is the selected file's Base64 payload, never a local path or a remote URL.
type AttachmentUpload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}
type pendingAttachment struct {
	ref  api.Attachment
	data []byte
}

func inspectImage(name string, data []byte) (api.InputImage, error) {
	if len(data) == 0 || len(data) > maxFileBytes {
		return api.InputImage{}, errors.New("图片为空或超过 8 MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return api.InputImage{}, errors.New("只支持有效的 PNG 或 JPEG 图片")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16<<20 {
		return api.InputImage{}, errors.New("图片尺寸过大，请缩小图片后重试")
	}
	return api.InputImage{Name: name, MIME: "image/" + format, Width: cfg.Width, Height: cfg.Height, Data: data}, nil
}
func validatedImage(name string, data []byte) (api.InputImage, error) {
	img, err := inspectImage(name, data)
	if err != nil {
		return img, err
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return api.InputImage{}, errors.New("图片数据不完整或已损坏")
	}
	return img, nil
}
func imageFile(chatID string, ref api.Attachment) (string, error) {
	if !id.Valid(chatID, id.Chat) || !id.Valid(ref.ID, id.Attachment) {
		return "", errors.New("无效的图片引用")
	}
	ext := ".png"
	if ref.MIME == "image/jpeg" {
		ext = ".jpg"
	} else if ref.MIME != "image/png" {
		return "", errors.New("不是可预览的图片")
	}
	return filepath.Join("attachments", chatID, ref.ID+ext), nil
}

// The encrypted scope survives moving a QQ client into the archive. Only host-issued IDs name files.
func imageScope(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	switch {
	case len(parts) == 4 && parts[0] == "chats":
		parts = parts[1:]
	case len(parts) >= 7 && parts[0] == "ext":
		if parts[1] == "archive" {
			parts = parts[1:]
		}
		if len(parts) != 8 || parts[1] != "clients" || parts[3] != "qq" {
			return ""
		}
		parts = parts[5:]
	default:
		return ""
	}
	if len(parts) != 3 || parts[0] != "attachments" || !id.Valid(parts[1], id.Chat) {
		return ""
	}
	ext := filepath.Ext(parts[2])
	if (ext != ".png" && ext != ".jpg") || !id.Valid(strings.TrimSuffix(parts[2], ext), id.Attachment) {
		return ""
	}
	return strings.Join(parts, "/")
}
func (c *Core) saveImage(directory, chatID string, ref api.Attachment, data []byte) error {
	rel, err := imageFile(chatID, ref)
	if err != nil {
		return err
	}
	if c.profileRoot != "" {
		data, err = c.vault.ChatBytes(filepath.ToSlash(rel), data, true)
		if err != nil {
			return err
		}
		defer clear(data)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.MkdirAll(filepath.Dir(rel), 0700); err != nil {
		return err
	}
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	err = errors.Join(err, f.Close())
	if err != nil {
		_ = root.Remove(rel)
	}
	return err
}
func boundedRead(rootPath, rel string, limit int64) ([]byte, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	fi, err := root.Stat(rel)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > limit {
		return nil, errors.New("请选择大小符合限制的普通文件")
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(b)) > limit {
		return nil, errors.New("文件超过大小限制")
	}
	return b, err
}
func (c *Core) loadImage(directory, chatID string, a api.Attachment) (api.InputImage, error) {
	rel, err := imageFile(chatID, a)
	if err != nil {
		return api.InputImage{}, err
	}
	b, err := boundedRead(directory, rel, maxFileBytes+128)
	if err != nil {
		return api.InputImage{}, err
	}
	if c.profileRoot != "" {
		b, err = c.vault.ChatBytes(filepath.ToSlash(rel), b, false)
		if err != nil {
			return api.InputImage{}, err
		}
	}
	img, err := inspectImage(a.Name, b)
	if err == nil && img.MIME != a.MIME {
		return api.InputImage{}, errors.New("图片类型不匹配")
	}
	return img, err
}
func imageRef(img api.InputImage) api.Attachment {
	return api.Attachment{ID: id.MustNew(id.Attachment), Name: img.Name, MIME: img.MIME}
}
func prepareUploads(files []AttachmentUpload, model api.Model) ([]pendingAttachment, error) {
	if len(files) > maxAttachments {
		return nil, errors.New("每条消息最多添加 4 个附件")
	}
	var out []pendingAttachment
	total := 0
	for _, f := range files {
		name := strings.TrimSpace(f.Name)
		if name == "" || len(name) > 255 || strings.ContainsAny(name, `/\`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return nil, errors.New("无效的附件名")
		}
		if len(f.Data) > base64.StdEncoding.EncodedLen(maxFileBytes) || len(f.Data) > base64.StdEncoding.EncodedLen(maxAttachmentBytes-total) {
			return nil, errors.New("附件过大：单个最多 8 MiB，合计最多 12 MiB")
		}
		b, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			return nil, errors.New("附件编码无效")
		}
		total += len(b)
		if total > maxAttachmentBytes || len(b) > maxFileBytes {
			return nil, errors.New("附件超过大小限制")
		}
		var a api.Attachment
		switch strings.ToLower(filepath.Ext(name)) {
		case ".png", ".jpg", ".jpeg":
			if !model.AllowsImages() {
				return nil, errors.New("当前模型不支持图片输入，请切换视觉模型后发送")
			}
			img, err := validatedImage(name, b)
			if err != nil {
				return nil, fmt.Errorf("%s：%w", name, err)
			}
			a = imageRef(img)
		case ".txt", ".md", ".log", ".json", ".csv", ".go", ".py", ".js", ".ts", ".vue", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".rs", ".c", ".cpp", ".h", ".java", ".sh", ".ps1", ".ini":
			if len(b) > maxTextBytes || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
				return nil, errors.New("文本附件需要是 256 KiB 以内的 UTF-8 文本")
			}
			a = api.Attachment{ID: id.MustNew(id.Attachment), Name: name, MIME: "text/plain", Text: strings.TrimPrefix(string(b), "\ufeff")}
			b = nil
		default:
			return nil, fmt.Errorf("暂不支持附件 %s，请选择 PNG、JPEG 或文本文件", name)
		}
		out = append(out, pendingAttachment{a, b})
	}
	return out, nil
}
func branchImages(s *chat.ChatTask) (map[string]api.Attachment, error) {
	path, err := s.Tree.CurrentPath()
	if err != nil {
		return nil, err
	}
	result := map[string]api.Attachment{}
	for _, n := range path {
		for _, m := range n.Child {
			for _, a := range m.Attachments {
				if a.IsImage() {
					result[a.ID] = a
				}
			}
			for _, call := range m.ToolCalls {
				if a := call.Image; a != nil {
					result[a.ID] = *a
				}
			}
		}
	}
	return result, nil
}
func (c *Core) GetChatImage(chatID, attachmentID string) (string, error) {
	session, err := c.GetChat(chatID)
	if err != nil {
		return "", err
	}
	refs, err := branchImages(session)
	if err != nil {
		return "", err
	}
	a, ok := refs[attachmentID]
	if !ok {
		return "", errors.New("当前对话分支没有这张图片")
	}
	directory, err := c.chatDirectory()
	if err != nil {
		return "", err
	}
	img, err := c.loadImage(directory, chatID, a)
	if err != nil {
		return "", err
	}
	return img.DataURL(), nil
}
