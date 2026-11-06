package Persona

import "encoding/json"

// Persona 是对话的人设：告诉模型「你是谁」。
//
// 存储与传输使用 JSON；ID 用于界面选择，Prompt 只发送角色内容。
type Persona struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	System      string `json:"system"`
}

const promptPrefix = "以下是你的角色设定 : " +
	"\n"

// Prompt 返回可直接作为 system 消息的文本。
func (p Persona) Prompt() (string, error) {
	data, err := json.Marshal(struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		System      string `json:"system"`
	}{
		Name:        p.Name,
		Description: p.Description,
		System:      p.System,
	})
	if err != nil {
		return "", err
	}
	return promptPrefix + string(data), nil
}

// DefaultID 是没有显式选择时使用的人设。
const DefaultID = "assistant"

// List 返回全部内置人设。
func List() []Persona {
	out := make([]Persona, len(builtins))
	copy(out, builtins)
	return out
}

// Find 按 ID 查找，找不到返回 nil。
func Find(id string) *Persona {
	for i := range builtins {
		if builtins[i].ID == id {
			persona := builtins[i]
			return &persona
		}
	}
	return nil
}

// Resolve 返回最终生效的人设：ID 为空或不存在时回落到默认。
func Resolve(id string) Persona {
	if persona := Find(id); persona != nil {
		return *persona
	}
	if persona := Find(DefaultID); persona != nil {
		return *persona
	}
	return builtins[0]
}
