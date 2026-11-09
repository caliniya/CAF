package Tests

import (
	"encoding/json"
	"slices"
	"testing"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	maintool "github.com/caliniya/caf/core/Agent/AgentTool/Tools/MainTool"
	backend "github.com/caliniya/caf/core/Backend"
)

func boolPtr(v bool) *bool { return &v }

func TestChatToolConfigEmptyOmitsFields(t *testing.T) {
	b, err := json.Marshal(&agenttool.ChatToolConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("空配置应序列化为 {}，实际 %s", b)
	}
}

func TestChatToolConfigSetAndRead(t *testing.T) {
	var cfg agenttool.ChatToolConfig
	cfg.SetKit("file", "read", boolPtr(true))
	cfg.SetKit("file", "write", boolPtr(false))

	if v, ok := cfg.Kit("file", "read"); !ok || !v {
		t.Fatalf("read 应为显式 true，实际 value=%v explicit=%v", v, ok)
	}
	if v, ok := cfg.Kit("file", "write"); !ok || v {
		t.Fatalf("write 应为显式 false，实际 value=%v explicit=%v", v, ok)
	}
	if _, ok := cfg.Kit("file", "missing"); ok {
		t.Fatal("未设置的 kit 应返回 explicit=false")
	}
	if _, ok := cfg.Kit("nope", "read"); ok {
		t.Fatal("未设置的 set 应返回 explicit=false")
	}
}

func TestChatToolConfigRoundTrip(t *testing.T) {
	var cfg agenttool.ChatToolConfig
	cfg.SetKit("file", "read", boolPtr(true))

	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"sets":{"file":{"kits":{"read":true}}}}` {
		t.Fatalf("序列化结果不符合契约：%s", data)
	}

	var got agenttool.ChatToolConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if v, ok := got.Kit("file", "read"); !ok || !v {
		t.Fatalf("反序列化后丢失设置：value=%v explicit=%v", v, ok)
	}
}

// 会话文件里可能出现 "sets": null 或 "kits": null，写回时不能 panic。
func TestChatToolConfigWriteAfterNullUnmarshal(t *testing.T) {
	var cfg agenttool.ChatToolConfig
	if err := json.Unmarshal([]byte(`{"sets":{"file":{"kits":null}}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.SetKit("file", "read", boolPtr(true))
	if v, ok := cfg.Kit("file", "read"); !ok || !v {
		t.Fatalf("nil map 写入失败：value=%v explicit=%v", v, ok)
	}
}

func TestChatToolConfigUnsetFallsBack(t *testing.T) {
	var cfg agenttool.ChatToolConfig
	cfg.SetKit("file", "read", boolPtr(true))
	cfg.SetKit("file", "read", nil)

	if _, ok := cfg.Kit("file", "read"); ok {
		t.Fatal("传 nil 应删除覆盖项")
	}
	if cfg.Sets != nil {
		t.Fatalf("清除唯一覆盖后 Sets 应回到 nil，实际 %#v", cfg.Sets)
	}

	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("清除后不应留下任何字段，实际 %s", data)
	}
}

func TestChatToolConfigNormalize(t *testing.T) {
	cfg := agenttool.ChatToolConfig{
		Sets: map[string]agenttool.ToolSetInfo{
			"file": {},
			"web":  {Kits: map[string]bool{"search": true}},
		},
	}
	cfg.Normalize()

	if _, ok := cfg.Sets["file"]; ok {
		t.Fatal("空 Kits 的 set 应被清掉")
	}
	if len(cfg.Sets) != 1 || !cfg.Sets["web"].Kits["search"] {
		t.Fatalf("非空 set 不应被误删：%#v", cfg.Sets)
	}
}

func TestToolSetDetailsIncludeParameterSchemas(t *testing.T) {
	useToolRegistry(t)
	set := maintool.InitMainTool()
	if err := agenttool.Add(set); err != nil {
		t.Fatal(err)
	}
	// 禁用工具也需要在模块详情中可见。
	agenttool.Tools["WriteFile"].Meta().Enabled = false
	sets := (&backend.Core{}).ListToolSets()
	if len(sets) != 1 || len(sets[0].Kits) != 1 {
		t.Fatalf("工具集和模块列表错误：%+v", sets)
	}
	kit := sets[0].Kits[0]
	wantIDs := []string{"ListDirectory", "ReadFile", "SearchFiles", "WriteFile"}
	if len(kit.Tools) != len(wantIDs) || !kit.DefaultEnabled {
		t.Fatalf("工具详情或模块启用状态错误：%+v", kit)
	}
	for index, tool := range kit.Tools {
		if tool.ID != wantIDs[index] || tool.Name == "" || tool.Description == "" {
			t.Fatalf("工具详情必须包含稳定顺序、名称和说明：%+v", tool)
		}
		properties := tool.Parameters["properties"].(map[string]any)
		if properties["path"].(map[string]any)["type"] != "array" {
			t.Fatalf("工具 %s 缺少路径参数类型", tool.ID)
		}
		timeout := properties["timeout_seconds"].(map[string]any)
		if timeout["type"] != "number" || timeout["default"] != 0 {
			t.Fatalf("详情缺少调用时可用的超时参数：%+v", timeout)
		}
		required := tool.Parameters["required"].([]string)
		if !slices.Contains(required, "path") || slices.Contains(required, "timeout_seconds") {
			t.Fatalf("必填和可选参数声明错误：%v", required)
		}
		if tool.ID == "WriteFile" && (!slices.Contains(required, "content") || properties["content"].(map[string]any)["type"] != "string") {
			t.Fatal("写入工具详情缺少必填的 content 参数")
		}
		originalProperties := agenttool.Tools[tool.ID].Meta().Parameters["properties"].(map[string]any)
		if _, exists := originalProperties["timeout_seconds"]; exists {
			t.Fatal("查询详情不应修改工具原始参数定义")
		}
	}
	data, err := json.Marshal(kit)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Tools []struct {
			ID          string         `json:"id"`
			Description string         `json:"description"`
			Parameters  map[string]any `json:"parameters"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Tools) != len(wantIDs) {
		t.Fatalf("工具详情 JSON 结构错误：%s，错误=%v", data, err)
	}
	if payload.Tools[0].ID != "ListDirectory" || payload.Tools[0].Description == "" || payload.Tools[0].Parameters["properties"] == nil {
		t.Fatalf("前端 JSON 缺少工具说明或参数：%s", data)
	}
}

func TestChatTaskOmitsNilToolConfig(t *testing.T) {
	b, err := json.Marshal(&struct {
		ToolConfig *agenttool.ChatToolConfig `json:"toolConfig,omitempty"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("nil ToolConfig 应整体省略，实际 %s", b)
	}
}
