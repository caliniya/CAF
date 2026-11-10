package Backend

import "encoding/json"

// Emit 由桌面宿主注入，Backend 不依赖 Wails。
// 未注入时为空操作,便于测试与其他宿主复用。
var Emit = func(event string, data ...any) {}

// 事件名,与前端 api.ts 中的契约一致。
const (
	EventChatDelta = "chat:delta" // 载荷为 JSON 字符串 {"content"?: string, "reasoning"?: string}
	EventChatTool  = "chat:tool"  // 载荷为 JSON 字符串，toolId/callId 用于识别，name 用于展示。
)

// EmitJSON 把结构体序列化成 JSON 字符串后推送——
// 前端的事件载荷契约是 JSON 字符串。
func EmitJSON(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	Emit(event, string(data))
}
