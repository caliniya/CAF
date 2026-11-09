package QQ

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

type QQMessageType string
type QQSegmentType string

const (
	QQMessagePrivate QQMessageType = "private"
	QQMessageGroup   QQMessageType = "group"

	QQSegmentText    QQSegmentType = "text"
	QQSegmentImage   QQSegmentType = "image"
	QQSegmentAt      QQSegmentType = "at"
	QQSegmentReply   QQSegmentType = "reply"
	QQSegmentFace    QQSegmentType = "face"
	QQSegmentRecord  QQSegmentType = "record"
	QQSegmentVideo   QQSegmentType = "video"
	QQSegmentFile    QQSegmentType = "file"
	QQSegmentForward QQSegmentType = "forward"
	QQSegmentNode    QQSegmentType = "node"
	QQSegmentJSON    QQSegmentType = "json"
	QQSegmentXML     QQSegmentType = "xml"

	// QQMessageTextPrefix 标记 QQ 消息的文本格式，后面是完整的 QQMessage JSON。
	// 模型消息仍只有 Role 和纯文本 Content，不需要携带 QQ 类型或附加字段。
	QQMessageTextPrefix = "[CAF.QQ.Message.v1]\n"
)

// QQMessage 是 CAF 内部的 QQ 消息，不依赖具体 QQ 客户端。
// ID 使用字符串，OneBot 适配时才转换为整数；Segments 保持消息原有顺序。
// UserID 是私聊对象或群消息所属用户，Sender 描述实际发送者。
type QQMessage struct {
	ID         string                     `json:"id,omitempty"`
	SelfID     string                     `json:"self_id,omitempty"`
	Type       QQMessageType              `json:"type,omitempty"`
	SubType    string                     `json:"sub_type,omitempty"`
	UserID     string                     `json:"user_id,omitempty"`
	GroupID    string                     `json:"group_id,omitempty"`
	Time       int64                      `json:"time,omitempty"`
	SentBySelf bool                       `json:"sent_by_self,omitempty"`
	Sender     *QQSender                  `json:"sender,omitempty"`
	Anonymous  *QQAnonymous               `json:"anonymous,omitempty"`
	Segments   []QQMessageSegment         `json:"segments"`
	RawMessage string                     `json:"raw_message,omitempty"`
	Font       int32                      `json:"font,omitempty"`
	Extra      map[string]json.RawMessage `json:"extra,omitempty"`
}

// QQMessageSegment 的 Type 可使用上面的常量，也允许客户端扩展类型。
// 常用数据键：text.text、image.file/url、at.qq（号码或 all）、reply.id。
// Data 原样保留图片地址、引用 ID、合并转发节点等结构，模型端仅看到其文本表示。
type QQMessageSegment struct {
	Type QQSegmentType  `json:"type"`
	Data map[string]any `json:"data"`
}

type QQSender struct {
	UserID   string                     `json:"user_id"`
	Nickname string                     `json:"nickname,omitempty"`
	Card     string                     `json:"card,omitempty"`
	Sex      string                     `json:"sex,omitempty"`
	Age      int                        `json:"age,omitempty"`
	Area     string                     `json:"area,omitempty"`
	Level    string                     `json:"level,omitempty"`
	Role     string                     `json:"role,omitempty"`
	Title    string                     `json:"title,omitempty"`
	Extra    map[string]json.RawMessage `json:"extra,omitempty"`
}

type QQAnonymous struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Flag string `json:"flag"`
}

func QQText(text string) QQMessageSegment {
	return QQMessageSegment{Type: QQSegmentText, Data: map[string]any{"text": text}}
}

// UnmarshalJSON 保留消息段扩展数据中的整数精度。
func (message *QQMessage) UnmarshalJSON(data []byte) error {
	type messageJSON QQMessage
	var value messageJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*message = QQMessage(value)
	return nil
}

// ToChatMessage 将 QQ 消息编码成带版本标记的纯文本。
// 外部消息使用 user，自身消息使用 assistant，不把外部正文升级成 system 指令。
func (message QQMessage) ToChatMessage() (apiImpl.ChatMessage, error) {
	if err := message.validate(); err != nil {
		return apiImpl.ChatMessage{}, err
	}
	data, err := json.MarshalIndent(message, "", "  ")
	if err != nil {
		return apiImpl.ChatMessage{}, fmt.Errorf("编码 QQ 消息文本失败：%w", err)
	}
	role := "user"
	if message.SentBySelf {
		role = "assistant"
	}
	return apiImpl.ChatMessage{Role: role, Content: QQMessageTextPrefix + string(data)}, nil
}

// QQMessageFromChatMessage 解析 QQ 文本格式，或把普通模型正文包装为 text 段。
// 普通正文不解析 CQ 码，因此模型输出 [CQ:...] 时不会意外发送图片或 @。
// 普通正文没有路由信息，发送前由调用方填写 Type、UserID 或 GroupID。
// Reasoning、ToolCalls 和 ToolCallID 不属于 QQ 正文，不参与转换。
func QQMessageFromChatMessage(message apiImpl.ChatMessage) (QQMessage, error) {
	if message.Role != "user" && message.Role != "assistant" {
		return QQMessage{}, fmt.Errorf("角色 %q 不能转换为 QQ 聊天消息", message.Role)
	}
	if !strings.HasPrefix(message.Content, QQMessageTextPrefix) {
		if message.Content == "" {
			return QQMessage{}, errors.New("QQ 消息正文不能为空")
		}
		return QQMessage{
			SentBySelf: message.Role == "assistant",
			Segments:   []QQMessageSegment{QQText(message.Content)},
		}, nil
	}
	var result QQMessage
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(message.Content, QQMessageTextPrefix)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return QQMessage{}, fmt.Errorf("解析 QQ 消息文本失败：%w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return QQMessage{}, errors.New("QQ 消息文本只能包含一个 JSON 对象")
	}
	if result.SentBySelf != (message.Role == "assistant") {
		return QQMessage{}, errors.New("QQ 消息的发送方向与模型消息角色不一致")
	}
	if err := result.validate(); err != nil {
		return QQMessage{}, err
	}
	return result, nil
}

func (message QQMessage) validate() error {
	if message.Type != "" && message.Type != QQMessagePrivate && message.Type != QQMessageGroup {
		return fmt.Errorf("不支持的 QQ 消息类型 %q", message.Type)
	}
	if len(message.Segments) == 0 {
		return errors.New("QQ 消息必须至少包含一个消息段")
	}
	for index, segment := range message.Segments {
		if segment.Type == "" || strings.ContainsAny(string(segment.Type), " \t\r\n,=[]") {
			return fmt.Errorf("QQ 消息段 %d 的 type 无效", index)
		}
		if segment.Data == nil {
			return fmt.Errorf("QQ 消息段 %d 的 data 必须是对象", index)
		}
		if segment.Type == QQSegmentText {
			if _, ok := segment.Data["text"].(string); !ok {
				return fmt.Errorf("QQ 文本消息段 %d 的 text 必须是字符串", index)
			}
		}
	}
	return nil
}

// ToQQMessage 将 OneBot 11 消息事件转换成内部消息，保留未知消息段和扩展字段。
func (event OneBot11MessageEvent) ToQQMessage() (QQMessage, error) {
	if event.PostType != OneBot11PostMessage && event.PostType != OneBot11PostMessageSent {
		return QQMessage{}, fmt.Errorf("OneBot 11 事件 %q 不是消息事件", event.PostType)
	}
	if event.MessageType != OneBot11MessagePrivate && event.MessageType != OneBot11MessageGroup {
		return QQMessage{}, fmt.Errorf("不支持的 OneBot 11 消息类型 %q", event.MessageType)
	}
	if event.SelfID <= 0 || event.UserID <= 0 {
		return QQMessage{}, errors.New("OneBot 11 消息的 self_id 和 user_id 必须为正整数")
	}
	if event.MessageType == OneBot11MessageGroup && event.GroupID <= 0 {
		return QQMessage{}, errors.New("OneBot 11 群消息必须包含有效的 group_id")
	}
	if err := validateOneBot11Segments(event.Message); err != nil {
		return QQMessage{}, err
	}
	message := QQMessage{
		ID: event.MessageID.String(), SelfID: event.SelfID.String(),
		Type: QQMessageType(event.MessageType), SubType: event.SubType,
		UserID: event.UserID.String(), Time: event.Time,
		SentBySelf: event.PostType == OneBot11PostMessageSent,
		RawMessage: event.RawMessage, Font: event.Font,
		Extra:    event.Extra,
		Segments: make([]QQMessageSegment, 0, len(event.Message)),
	}
	if event.GroupID != 0 {
		message.GroupID = event.GroupID.String()
	}
	if event.Sender != nil {
		message.Sender = &QQSender{
			UserID: event.Sender.UserID.String(), Nickname: event.Sender.Nickname,
			Card: event.Sender.Card, Sex: event.Sender.Sex, Age: event.Sender.Age,
			Area: event.Sender.Area, Level: event.Sender.Level, Role: event.Sender.Role,
			Title: event.Sender.Title, Extra: event.Sender.Extra,
		}
	}
	if event.Anonymous != nil {
		message.Anonymous = &QQAnonymous{
			ID: event.Anonymous.ID.String(), Name: event.Anonymous.Name, Flag: event.Anonymous.Flag,
		}
	}
	for _, segment := range event.Message {
		message.Segments = append(message.Segments, QQMessageSegment{
			Type: QQSegmentType(segment.Type), Data: segment.Data,
		})
	}
	if err := message.validate(); err != nil {
		return QQMessage{}, err
	}
	return cloneQQValue(message)
}

// ToOneBot11MessageEvent 生成消息事件。已接收/已发送的事件可用于协议互转，
// 真正发送新消息应使用 ToOneBot11Action，不伪造消息 ID。
func (message QQMessage) ToOneBot11MessageEvent() (OneBot11MessageEvent, error) {
	if err := message.validate(); err != nil {
		return OneBot11MessageEvent{}, err
	}
	if message.Type != QQMessagePrivate && message.Type != QQMessageGroup {
		return OneBot11MessageEvent{}, errors.New("生成 OneBot 11 事件前必须指定私聊或群聊类型")
	}
	selfID, err := qqOneBotID("self_id", message.SelfID, true)
	if err != nil {
		return OneBot11MessageEvent{}, err
	}
	userID, err := qqOneBotID("user_id", message.UserID, true)
	if err != nil {
		return OneBot11MessageEvent{}, err
	}
	groupID, err := qqOneBotID("group_id", message.GroupID, message.Type == QQMessageGroup)
	if err != nil {
		return OneBot11MessageEvent{}, err
	}
	if message.ID == "" {
		return OneBot11MessageEvent{}, errors.New("生成 OneBot 11 事件需要原有消息 ID；发送新消息请使用 ToOneBot11Action")
	}
	messageID, err := qqOneBotID("message_id", message.ID, false)
	if err != nil {
		return OneBot11MessageEvent{}, err
	}
	postType := OneBot11PostMessage
	if message.SentBySelf {
		postType = OneBot11PostMessageSent
	}
	event := OneBot11MessageEvent{
		OneBot11EventHeader: OneBot11EventHeader{Time: message.Time, SelfID: selfID, PostType: postType},
		MessageType:         string(message.Type), SubType: message.SubType,
		MessageID: messageID, UserID: userID, GroupID: groupID,
		RawMessage: message.RawMessage, Font: message.Font, Extra: message.Extra,
	}
	if message.Sender != nil {
		senderID, err := qqOneBotID("sender.user_id", message.Sender.UserID, false)
		if err != nil {
			return OneBot11MessageEvent{}, err
		}
		event.Sender = &OneBot11Sender{
			UserID: senderID, Nickname: message.Sender.Nickname,
			Card: message.Sender.Card, Sex: message.Sender.Sex, Age: message.Sender.Age,
			Area: message.Sender.Area, Level: message.Sender.Level, Role: message.Sender.Role,
			Title: message.Sender.Title, Extra: message.Sender.Extra,
		}
	}
	if message.Anonymous != nil {
		anonymousID, err := qqOneBotID("anonymous.id", message.Anonymous.ID, false)
		if err != nil {
			return OneBot11MessageEvent{}, err
		}
		event.Anonymous = &OneBot11Anonymous{
			ID: anonymousID, Name: message.Anonymous.Name, Flag: message.Anonymous.Flag,
		}
	}
	segments, err := message.oneBot11Segments()
	if err != nil {
		return OneBot11MessageEvent{}, err
	}
	event.Message = segments
	return cloneQQValue(event)
}

// ToOneBot11Action 只构造发送请求，不连接客户端也不实际发送。
// 消息使用数组格式，纯文本中的 CQ 字样不会被客户端重新解释。
func (message QQMessage) ToOneBot11Action(echo json.RawMessage) (OneBot11ActionRequest, error) {
	if err := message.validate(); err != nil {
		return OneBot11ActionRequest{}, err
	}
	segments, err := message.oneBot11Segments()
	if err != nil {
		return OneBot11ActionRequest{}, err
	}
	params := map[string]any{"message": segments}
	action := ""
	switch message.Type {
	case QQMessagePrivate:
		id, err := qqOneBotID("user_id", message.UserID, true)
		if err != nil {
			return OneBot11ActionRequest{}, err
		}
		action = "send_private_msg"
		params["user_id"] = id
	case QQMessageGroup:
		id, err := qqOneBotID("group_id", message.GroupID, true)
		if err != nil {
			return OneBot11ActionRequest{}, err
		}
		action = "send_group_msg"
		params["group_id"] = id
	default:
		return OneBot11ActionRequest{}, errors.New("发送前必须指定私聊或群聊类型")
	}
	if len(echo) != 0 && !json.Valid(echo) {
		return OneBot11ActionRequest{}, errors.New("echo 必须是有效的 JSON")
	}
	return OneBot11ActionRequest{
		Action: action, Params: params, Echo: bytes.Clone(echo),
	}, nil
}

// QQMessageFromOneBot11Action 解析发送动作，返回尚未发送的内部消息。
// 支持 send_msg、send_private_msg、send_group_msg；不补造账号、时间或消息 ID。
func QQMessageFromOneBot11Action(request OneBot11ActionRequest) (QQMessage, error) {
	data, err := json.Marshal(request.Params)
	if err != nil {
		return QQMessage{}, err
	}
	var params struct {
		MessageType string          `json:"message_type"`
		UserID      OneBot11ID      `json:"user_id"`
		GroupID     OneBot11ID      `json:"group_id"`
		Message     json.RawMessage `json:"message"`
		AutoEscape  bool            `json:"auto_escape"`
	}
	if err := json.Unmarshal(data, &params); err != nil {
		return QQMessage{}, fmt.Errorf("解析 OneBot 11 发送参数失败：%w", err)
	}
	messageType := params.MessageType
	switch request.Action {
	case "send_private_msg":
		if messageType != "" && messageType != OneBot11MessagePrivate {
			return QQMessage{}, errors.New("send_private_msg 的 message_type 不一致")
		}
		messageType = OneBot11MessagePrivate
	case "send_group_msg":
		if messageType != "" && messageType != OneBot11MessageGroup {
			return QQMessage{}, errors.New("send_group_msg 的 message_type 不一致")
		}
		messageType = OneBot11MessageGroup
	case "send_msg":
		if messageType == "" {
			switch {
			case params.UserID > 0 && params.GroupID == 0:
				messageType = OneBot11MessagePrivate
			case params.GroupID > 0 && params.UserID == 0:
				messageType = OneBot11MessageGroup
			default:
				return QQMessage{}, errors.New("send_msg 必须明确指定消息类型或唯一的接收对象")
			}
		}
	default:
		return QQMessage{}, fmt.Errorf("动作 %q 不是 OneBot 11 发送消息动作", request.Action)
	}
	result := QQMessage{Type: QQMessageType(messageType), SentBySelf: true}
	switch result.Type {
	case QQMessagePrivate:
		if params.UserID <= 0 {
			return QQMessage{}, errors.New("私聊发送需要有效的 user_id")
		}
		result.UserID = params.UserID.String()
	case QQMessageGroup:
		if params.GroupID <= 0 {
			return QQMessage{}, errors.New("群消息发送需要有效的 group_id")
		}
		result.GroupID = params.GroupID.String()
	default:
		return QQMessage{}, fmt.Errorf("不支持的发送消息类型 %q", messageType)
	}
	messageData := bytes.TrimSpace(params.Message)
	if len(messageData) == 0 || bytes.Equal(messageData, []byte("null")) {
		return QQMessage{}, errors.New("发送参数缺少 message")
	}
	var segments OneBot11Message
	if params.AutoEscape && messageData[0] == '"' {
		var text string
		if err := json.Unmarshal(messageData, &text); err != nil {
			return QQMessage{}, err
		}
		result.Segments = []QQMessageSegment{QQText(text)}
	} else {
		if err := json.Unmarshal(messageData, &segments); err != nil {
			return QQMessage{}, err
		}
		result.Segments = make([]QQMessageSegment, 0, len(segments))
		for _, segment := range segments {
			result.Segments = append(result.Segments, QQMessageSegment{
				Type: QQSegmentType(segment.Type), Data: segment.Data,
			})
		}
	}
	if err := result.validate(); err != nil {
		return QQMessage{}, err
	}
	return result, nil
}

func (message QQMessage) oneBot11Segments() (OneBot11Message, error) {
	result := make(OneBot11Message, 0, len(message.Segments))
	for _, segment := range message.Segments {
		result = append(result, OneBot11MessageSegment{Type: string(segment.Type), Data: segment.Data})
	}
	return cloneQQValue(result)
}

func qqOneBotID(name, text string, positive bool) (OneBot11ID, error) {
	if text == "" && !positive {
		return 0, nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s 必须是有效的 64 位整数：%w", name, err)
	}
	if positive && value <= 0 {
		return 0, fmt.Errorf("%s 必须是正整数", name)
	}
	return OneBot11ID(value), nil
}

// cloneQQValue 深拷贝扩展和消息段数据，并保留 JSON 整数精度。
func cloneQQValue[T any](value T) (T, error) {
	var result T
	data, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err = decoder.Decode(&result)
	return result, err
}
