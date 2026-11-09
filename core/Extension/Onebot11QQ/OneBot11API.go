package Onebot11QQ

import "encoding/json"

// OneBot11ID 使用有符号 64 位整数；消息 ID 允许负数。
// JSON 解码同时接受整数和整数字符串，避免通过 float64 丢失精度。
type OneBot11ID int64

const (
	OneBot11PostMessage     = "message"
	OneBot11PostMessageSent = "message_sent" // 部分实现扩展的自身消息事件。
	OneBot11PostNotice      = "notice"
	OneBot11PostRequest     = "request"
	OneBot11PostMetaEvent   = "meta_event"

	OneBot11MessagePrivate = "private"
	OneBot11MessageGroup   = "group"
)

// OneBot11MessageSegment 是 OneBot 11 的消息段。
// Data 保留实现扩展及 node 等消息段中的嵌套内容。
type OneBot11MessageSegment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// OneBot11Message 兼容消息段数组和 CQ 码字符串；编码统一输出消息段数组。
type OneBot11Message []OneBot11MessageSegment

type OneBot11EventHeader struct {
	Time     int64      `json:"time"`
	SelfID   OneBot11ID `json:"self_id"`
	PostType string     `json:"post_type"`
}

type OneBot11MessageEvent struct {
	OneBot11EventHeader
	MessageType string             `json:"message_type"`
	SubType     string             `json:"sub_type,omitempty"`
	MessageID   OneBot11ID         `json:"message_id"`
	UserID      OneBot11ID         `json:"user_id"`
	GroupID     OneBot11ID         `json:"group_id,omitempty"`
	Message     OneBot11Message    `json:"message"`
	RawMessage  string             `json:"raw_message,omitempty"`
	Font        int32              `json:"font,omitempty"`
	Sender      *OneBot11Sender    `json:"sender,omitempty"`
	Anonymous   *OneBot11Anonymous `json:"anonymous,omitempty"`
	// Extra 保存具体实现添加的事件字段。
	Extra map[string]json.RawMessage `json:"-"`
}

type OneBot11Sender struct {
	UserID   OneBot11ID                 `json:"user_id"`
	Nickname string                     `json:"nickname,omitempty"`
	Sex      string                     `json:"sex,omitempty"`
	Age      int                        `json:"age,omitempty"`
	Card     string                     `json:"card,omitempty"`
	Area     string                     `json:"area,omitempty"`
	Level    string                     `json:"level,omitempty"`
	Role     string                     `json:"role,omitempty"`
	Title    string                     `json:"title,omitempty"`
	Extra    map[string]json.RawMessage `json:"-"`
}

type OneBot11Anonymous struct {
	ID   OneBot11ID `json:"id"`
	Name string     `json:"name"`
	Flag string     `json:"flag"`
}

// OneBot11NoticeEvent 覆盖群成员、管理员、禁言、撤回、戳一戳等常见通知。
type OneBot11NoticeEvent struct {
	OneBot11EventHeader
	NoticeType string        `json:"notice_type"`
	SubType    string        `json:"sub_type,omitempty"`
	GroupID    OneBot11ID    `json:"group_id,omitempty"`
	UserID     OneBot11ID    `json:"user_id,omitempty"`
	OperatorID OneBot11ID    `json:"operator_id,omitempty"`
	TargetID   OneBot11ID    `json:"target_id,omitempty"`
	MessageID  OneBot11ID    `json:"message_id,omitempty"`
	Duration   int64         `json:"duration,omitempty"`
	HonorType  string        `json:"honor_type,omitempty"`
	File       *OneBot11File `json:"file,omitempty"`
}

type OneBot11File struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	BusID int64  `json:"busid,omitempty"`
}

type OneBot11RequestEvent struct {
	OneBot11EventHeader
	RequestType string     `json:"request_type"`
	SubType     string     `json:"sub_type,omitempty"`
	GroupID     OneBot11ID `json:"group_id,omitempty"`
	UserID      OneBot11ID `json:"user_id"`
	Comment     string     `json:"comment,omitempty"`
	Flag        string     `json:"flag"`
}

type OneBot11MetaEvent struct {
	OneBot11EventHeader
	MetaEventType string          `json:"meta_event_type"`
	SubType       string          `json:"sub_type,omitempty"`
	Interval      int64           `json:"interval,omitempty"`
	Status        json.RawMessage `json:"status,omitempty"`
}

// OneBot11ActionRequest 是 HTTP / WebSocket 动作请求，不包含传输逻辑。
// Echo 使用原始 JSON，允许字符串、数字等任意关联值。
type OneBot11ActionRequest struct {
	Action string          `json:"action"`
	Params map[string]any  `json:"params"`
	Echo   json.RawMessage `json:"echo,omitempty"`
}

type OneBot11ActionResponse struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Echo    json.RawMessage `json:"echo,omitempty"`
	Message string          `json:"message,omitempty"` // 实现扩展的错误说明。
	Wording string          `json:"wording,omitempty"`
}

type OneBot11SendMessageResult struct {
	MessageID OneBot11ID `json:"message_id"`
}
