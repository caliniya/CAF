package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	netclient "github.com/caliniya/caf/NetClient"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// Emit 由后端(desktop)注入的事件推送实现,core 不依赖 wails。
// 未注入时为空操作,便于测试与其他宿主复用。
var Emit = func(event string, data ...any) {}

// 事件名,与前端 api.ts 中的契约一致。
const (
	EventChatDelta = "chat:delta" // 载荷为 JSON 字符串 {"content"?: string, "reasoning"?: string}
	EventChatTool  = "chat:tool"  // 载荷为 JSON 字符串 {"name": string, "event": "start"|"done"}
)

// deltaPayload 是 chat:delta 的事件载荷。
type deltaPayload struct {
	Content   string `json:"content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

// SendMessage 供前端调用:用当前启用的配置发起一次流式对话。
// history 由前端传入(user/assistant 交替),流式增量通过 chat:delta 事件推送;
// 函数返回即代表对话结束,返回 error 表示失败。
//
// 每条请求都新建客户端与协议实例:协议实例持有 tool_call 分片聚合状态,
// 共享会让并行对话互相污染。
func (c *Core) SendMessage(history []apiImpl.ChatMessage) error {
	if c == nil || c.Configs == nil || c.Configs.EnablesConfig == nil {
		return errors.New("尚未选择配置（提供商），请先选择配置")
	}
	cfg := c.Configs.EnablesConfig
	if cfg.EnabledModel == "" {
		return errors.New("尚未选定模型，请先选择模型")
	}
	if len(history) == 0 {
		return errors.New("消息为空")
	}
	// 只接受 user/assistant 的对话历史,避免前端传入非法角色。
	for i := range history {
		role := history[i].Role
		if role != "user" && role != "assistant" {
			return fmt.Errorf("不支持的消息角色：%s", role)
		}
	}

	client := netclient.NewAPIClient(netclient.Options{
		ServerAddress: cfg.ServerAddress,
		APIKey:        cfg.APIKey,
		Model:         cfg.EnabledModel,
	})
	proto := &apiImpl.ChatCompletionAPI{}

	req := apiImpl.ChatRequest{
		Model:    cfg.EnabledModel,
		Messages: history,
	}

	ctx := context.Background()
	return client.StreamChat(ctx, req, proto, func(d apiImpl.Delta) {
		if d.Done {
			return
		}
		if d.Content != "" || d.Reasoning != "" {
			EmitJSON(EventChatDelta, deltaPayload{Content: d.Content, Reasoning: d.Reasoning})
		}
		if len(d.ToolCalls) > 0 {
			// 工具执行尚未接入,先把调用声明推给前端展示。
			for _, tc := range d.ToolCalls {
				EmitJSON(EventChatTool, map[string]string{"name": tc.Name, "event": "start"})
				EmitJSON(EventChatTool, map[string]string{"name": tc.Name, "event": "done"})
			}
		}
	})
}

// EmitJSON 把结构体序列化成 JSON 字符串后推送——
// 前端的事件载荷契约是 JSON 字符串。
func EmitJSON(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	Emit(event, string(data))
}
