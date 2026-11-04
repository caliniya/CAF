package tests

import (
	"encoding/json"
	"testing"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
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
