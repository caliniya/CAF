package Persona

import "encoding/json"

// Persona 是对话的人设：告诉模型「你是谁」。
//
// 存储与传输都用 JSON 本身——发送时直接 Marshal 成 system 内容，
// 字段名保持短，减少 token 开销。未来支持外部加载时，
// 同一套 json 标签可以直接反序列化，不需要改结构。
type Persona struct {
	ID          string   `json:"-"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	System      string   `json:"system"`
	Personality []string `json:"personality,omitempty"`
	Style       string   `json:"style,omitempty"`
	Scenario    string   `json:"scenario,omitempty"`
}

// promptPrefix 给 JSON 一个任务框架，否则模型可能把它当成参考资料而不是身份。
const promptPrefix = "以下是你的角色设定。请完全以这个角色的身份、性格和说话方式回复，" +
	"不要复述或解释设定本身:\n"

// Prompt 返回可直接作为 system 消息的文本。
func (p Persona) Prompt() (string, error) {
	data, err := json.Marshal(p)
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
