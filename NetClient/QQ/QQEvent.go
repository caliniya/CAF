package QQ

import (
	"encoding/json"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

type EventType string

const (
	EventMessage EventType = "message"
	EventRequest EventType = "request"
	EventNotice  EventType = "notice"
	EventStatus  EventType = "status"
	EventError   EventType = "error"
)

// Event 使用内部 QQ 数据；原始通知保存在 Raw 中，方便以后支持更多事件。
// 好友和群申请只上报，不自动批准，也不在适配器中触发模型或通知主人。
type Event struct {
	Type    EventType          `json:"type"`
	SelfID  string             `json:"self_id"`
	Time    int64              `json:"time"`
	Message *apiImpl.QQMessage `json:"message,omitempty"`
	Request *Request           `json:"request,omitempty"`
	Notice  *Notice            `json:"notice,omitempty"`
	Info    *ClientInfo        `json:"info,omitempty"`
	Error   string             `json:"error,omitempty"`
	Raw     json.RawMessage    `json:"raw,omitempty"`
}

type Request struct {
	Type    string `json:"type"` // friend 或 group
	SubType string `json:"sub_type,omitempty"`
	UserID  string `json:"user_id"`
	GroupID string `json:"group_id,omitempty"`
	Comment string `json:"comment,omitempty"`
	Flag    string `json:"flag"`
}

type Notice struct {
	Type       string `json:"type"`
	SubType    string `json:"sub_type,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	OperatorID string `json:"operator_id,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
}
