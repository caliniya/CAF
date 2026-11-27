package APIImpl

import (
	"encoding/base64"
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
		return true
	} // many compatible endpoints omit capabilities
	for _, v := range m.InputModalities {
		if strings.EqualFold(strings.TrimSpace(v), "image") {
			return true
		}
	}
	return false
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
