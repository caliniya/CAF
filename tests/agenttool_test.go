package Tests

import (
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	maintool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
)

type listEnableTestTool struct {
	meta *agenttool.ToolMeta
}

func (t *listEnableTestTool) Load() {}

func (t *listEnableTestTool) Meta() *agenttool.ToolMeta {
	return t.meta
}

func (t *listEnableTestTool) Fun(*agenttool.ToolContext, map[string]any) (string, error) {
	return "", nil
}

func (t *listEnableTestTool) ToType() *apiImpl.Tool {
	return &apiImpl.Tool{}
}

func TestListEnable(t *testing.T) {
	previousTools := agenttool.Tools
	t.Cleanup(func() { agenttool.Tools = previousTools })

	enabledTool := &listEnableTestTool{meta: &agenttool.ToolMeta{ID: "enabled", Name: "启用工具", Enabled: true}}
	agenttool.Tools = map[string]agenttool.Tool{
		"disabled": &listEnableTestTool{meta: &agenttool.ToolMeta{ID: "disabled", Name: "禁用工具"}},
		"enabled":  enabledTool,
		"nil-meta": &listEnableTestTool{},
		"nil-tool": nil,
	}

	got := agenttool.ListEnable()
	if len(got) != 1 || got[0] != enabledTool {
		t.Fatalf("ListEnable() = %#v, want only enabled tool", got)
	}
}

func TestListEnableWithoutTools(t *testing.T) {
	previousTools := agenttool.Tools
	t.Cleanup(func() { agenttool.Tools = previousTools })

	agenttool.Tools = nil
	if got := agenttool.ListEnable(); got != nil {
		t.Fatalf("ListEnable() = %#v, want nil", got)
	}
}

func useToolRegistry(t *testing.T) {
	t.Helper()
	previousTools, previousSets, previousList := agenttool.Tools, agenttool.ToolSets, agenttool.ToolList
	t.Cleanup(func() {
		agenttool.Tools, agenttool.ToolSets, agenttool.ToolList = previousTools, previousSets, previousList
	})
	agenttool.Init()
}

func TestToolRegistryUsesIDAndAllowsSharedDisplayNames(t *testing.T) {
	useToolRegistry(t)
	for _, id := range []string{"ReadFile", "OtherReadFile"} {
		set := maintool.InitMainTool()
		set.Name = id + "Set"
		kit := set.Get("文件操作工具")
		set.ToolKits = map[string]agenttool.ToolKit{"文件操作工具": kit}
		tool := kit.ListName()["ReadFile"]
		tool.Meta().ID = id
		// 此测试只注册一个工具，隔离重复展示名和 API ID 的行为。
		for existingID := range kit.ListName() {
			delete(kit.ListName(), existingID)
		}
		kit.ListName()[id] = tool
		if err := agenttool.Add(set); err != nil {
			t.Fatal(err)
		}
		if agenttool.Tools[id] != tool || agenttool.ToolList[id].Name != id || tool.ToType().Name != id {
			t.Fatalf("工具注册和请求必须使用 ID %q", id)
		}
		if got := agenttool.DisplayName(id); got != "读取文件" {
			t.Fatalf("DisplayName(%q) = %q", id, got)
		}
	}
	if _, exists := agenttool.Tools["读取文件"]; exists {
		t.Fatal("工具展示名称不能作为注册键")
	}
	if got := agenttool.DisplayName("unknown"); got != "unknown" {
		t.Fatalf("未知工具应回退到 ID，实际 %q", got)
	}
}

func TestToolRegistryRejectsInvalidIDs(t *testing.T) {
	for _, id := range []string{"", "读取文件", "read file", strings.Repeat("a", 65)} {
		t.Run(id, func(t *testing.T) {
			useToolRegistry(t)
			set := maintool.InitMainTool()
			set.Get("文件操作工具").ListName()["ReadFile"].Meta().ID = id
			if err := agenttool.Add(set); err == nil {
				t.Fatalf("应拒绝无效 ID %q", id)
			}
			if len(agenttool.Tools) != 0 || len(agenttool.ToolSets) != 0 || len(agenttool.ToolList) != 0 {
				t.Fatal("注册失败不应留下工具或工具集")
			}
		})
	}
}

func TestToolRegistryRejectsDuplicateIDs(t *testing.T) {
	useToolRegistry(t)
	set := maintool.InitMainTool()
	if err := agenttool.Add(set); err != nil {
		t.Fatal(err)
	}
	duplicate := maintool.InitMainTool()
	duplicate.Name = "另一个工具集"
	duplicate.Get("文件操作工具").ListName()["ReadFile"].Meta().Name = "另一个读取文件工具"
	if err := agenttool.Add(duplicate); err == nil {
		t.Fatal("展示名称不同也不能注册相同的工具 ID")
	}
	count := 0
	for _, kit := range set.List() {
		count += len(kit.List())
	}
	if len(agenttool.Tools) != count || len(agenttool.ToolSets) != 1 || len(agenttool.ToolList) != len(agenttool.Tools) {
		t.Fatal("重复 ID 注册失败不应修改工具表")
	}
}
