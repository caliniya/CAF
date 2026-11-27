package APIImpl

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MIME string `json:"mime"`
	Text string `json:"text,omitempty"`
}

func (a Attachment) IsImage() bool { return a.MIME == "image/png" || a.MIME == "image/jpeg" }

type InputImage struct {
	Name, MIME    string
	Data          []byte
	Width, Height int
}

func (i InputImage) DataURL() string {
	return "data:" + i.MIME + ";base64," + base64.StdEncoding.EncodeToString(i.Data)
}
func (m Model) AllowsImages() bool {
	if len(m.InputModalities) == 0 {
		return m.VisionConfirmed
	}
	for _, v := range m.InputModalities {
		if strings.EqualFold(strings.TrimSpace(v), "image") {
			return true
		}
	}
	return false
}

func (m Model) CheckImageInput() error {
	if m.AllowsImages() {
		return nil
	}
	if len(m.InputModalities) == 0 {
		return errors.New("尚未确认当前模型支持图片输入，请在模型供应商设置中确认视觉能力，或切换到带「视觉」标签的模型")
	}
	return errors.New("当前模型不支持图片输入，请切换到带「视觉」标签的模型")
}

func (m ChatMessage) InputText() string {
	if m.Role != "user" || len(m.Attachments) == 0 {
		return m.Content
	}
	var b strings.Builder
	b.WriteString(m.Content)
	for _, a := range m.Attachments {
		if a.MIME == "text/plain" {
			fmt.Fprintf(&b, "\n[文本附件 %q]\n%s\n[附件结束]", a.Name, a.Text)
		} else {
			fmt.Fprintf(&b, "\n[图片附件 %q，编号 %s]", a.Name, a.ID)
		}
	}
	return b.String()
}
