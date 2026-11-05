package Tests

import (
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type listEnableTestTool struct {
	meta *agenttool.ToolMeta
}

func (t *listEnableTestTool) Load() {}

func (t *listEnableTestTool) Meta() *agenttool.ToolMeta {
	return t.meta
}

func (t *listEnableTestTool) Fun(map[string]any) (string, error) {
	return "", nil
}

func (t *listEnableTestTool) ToType() apiImpl.Tool {
	return apiImpl.Tool{}
}

func TestListEnable(t *testing.T) {
	previousTools := agenttool.Tools
	t.Cleanup(func() { agenttool.Tools = previousTools })

	enabledTool := &listEnableTestTool{meta: &agenttool.ToolMeta{Name: "enabled", Enabled: true}}
	agenttool.Tools = map[string]agenttool.Tool{
		"disabled": &listEnableTestTool{meta: &agenttool.ToolMeta{Name: "disabled"}},
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
