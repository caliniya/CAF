package AgentTool

// ChatToolConfig 是工具启停策略：纯数据、可序列化。
// 同一类型用于两处——ChatTask.ToolConfig（对话级）与 Configs.ToolNormalConfig（全局默认）。
//
// 键不存在表示"继承上一层"，因此只写入显式设置的项：
// 对话级缺失 → 查全局默认；全局缺失 → 用 kit 的代码默认值。
type ChatToolConfig struct {
	Sets map[string]ToolSetInfo `json:"sets,omitempty"`
}

type ToolSetInfo struct {
	Kits map[string]bool `json:"kits,omitempty"`
}

// Kit 读取某个 kit 的显式设置；explicit 为 false 表示没有覆盖，调用方应继续向上一层查找。
func (c *ChatToolConfig) Kit(setID, kitID string) (value bool, explicit bool) {
	if c == nil || c.Sets == nil {
		return false, false
	}
	set, ok := c.Sets[setID]
	if !ok || set.Kits == nil {
		return false, false
	}
	value, explicit = set.Kits[kitID]
	return value, explicit
}

// SetKit 写入显式设置；on 为 nil 时删除该项，回退到继承。
func (c *ChatToolConfig) SetKit(setID, kitID string, on *bool) {
	if c == nil {
		return
	}
	if on == nil {
		c.remove(setID, kitID)
		return
	}
	if c.Sets == nil {
		c.Sets = make(map[string]ToolSetInfo)
	}
	// ToolSetInfo 是值类型，从 map 取出的是拷贝，改完必须写回。
	set := c.Sets[setID]
	if set.Kits == nil {
		set.Kits = make(map[string]bool)
	}
	set.Kits[kitID] = *on
	c.Sets[setID] = set
}

func (c *ChatToolConfig) remove(setID, kitID string) {
	if c.Sets == nil {
		return
	}
	set, ok := c.Sets[setID]
	if !ok {
		return
	}
	delete(set.Kits, kitID)
	if len(set.Kits) == 0 {
		delete(c.Sets, setID)
		if len(c.Sets) == 0 {
			c.Sets = nil
		}
		return
	}
	c.Sets[setID] = set
}

// Normalize 清掉空壳（空 Kits、空 Sets），保证"没有任何覆盖"时序列化不产生字段。
func (c *ChatToolConfig) Normalize() {
	if c == nil || c.Sets == nil {
		return
	}
	for setID, set := range c.Sets {
		if len(set.Kits) == 0 {
			delete(c.Sets, setID)
		}
	}
	if len(c.Sets) == 0 {
		c.Sets = nil
	}
}
