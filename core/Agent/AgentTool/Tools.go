package AgentTool

import (
	"errors"
	"fmt"
	"regexp"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// Tools 和 ToolList 按 ToolMeta.ID 索引；Name 仅用于展示。
var Tools map[string]Tool
var ToolSets map[string]ToolsSet
var ToolList map[string]apiImpl.Tool

var toolIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func Init() {
	Tools = map[string]Tool{}
	ToolSets = map[string]ToolsSet{}
	ToolList = map[string]apiImpl.Tool{}
}

func Add(t ToolsSet) error {
	if ToolSets == nil {
		return errors.New("要加载的工具集为nil")
	}

	pending := make(map[string]Tool)
	for _, kit := range t.Meta().ToolKits {
		for _, tool := range kit.List() {
			meta := tool.Meta()
			if !toolIDPattern.MatchString(meta.ID) {
				return fmt.Errorf("工具 %q 的 ID 无效：只能使用 1 至 64 个英文字母、数字、下划线或连字符", meta.Name)
			}

			if _, exists := Tools[meta.ID]; exists {
				return errors.New("重复的工具 ID: " + meta.ID)
			}
			if _, exists := pending[meta.ID]; exists {
				return errors.New("重复的工具 ID: " + meta.ID)
			}

			pending[meta.ID] = tool
		}
	}
	ToolSets[t.Meta().Name] = t
	for id, tool := range pending {
		Tools[id] = tool
	}
	Sync()
	return nil
}

func Sync() {
	ToolList = make(map[string]apiImpl.Tool, len(Tools))
	for id, tool := range Tools {
		meta := tool.Meta()
		ToolList[id] = *meta.ToType()
	}
}

// DisplayName 把 API 返回的工具 ID 转成界面名称；未知工具保留 ID。
func DisplayName(id string) string {
	if tool := Tools[id]; tool != nil {
		if meta := tool.Meta(); meta != nil && meta.Name != "" {
			return meta.Name
		}
	}
	return id
}
