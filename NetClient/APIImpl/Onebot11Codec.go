package APIImpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (id OneBot11ID) String() string {
	return strconv.FormatInt(int64(id), 10)
}

func (id *OneBot11ID) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("OneBot 11 ID 必须是 64 位整数或整数字符串：%w", err)
	}
	*id = OneBot11ID(value)
	return nil
}

func (message *OneBot11Message) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*message = nil
		return nil
	}
	if len(data) == 0 {
		return errors.New("OneBot 11 消息不能为空 JSON")
	}
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		segments, err := ParseOneBot11CQMessage(text)
		if err != nil {
			return err
		}
		*message = segments
		return nil
	}
	if data[0] != '[' {
		return errors.New("OneBot 11 消息必须是消息段数组或 CQ 码字符串")
	}
	var segments []OneBot11MessageSegment
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&segments); err != nil {
		return err
	}
	if err := validateOneBot11Segments(segments); err != nil {
		return err
	}
	*message = segments
	return nil
}

func (message OneBot11Message) MarshalJSON() ([]byte, error) {
	if message == nil {
		return []byte("[]"), nil
	}
	if err := validateOneBot11Segments(message); err != nil {
		return nil, err
	}
	return json.Marshal([]OneBot11MessageSegment(message))
}

func validateOneBot11Segments(segments []OneBot11MessageSegment) error {
	for index, segment := range segments {
		if segment.Type == "" || strings.ContainsAny(segment.Type, " \t\r\n,=[]") {
			return fmt.Errorf("消息段 %d 的 type 无效", index)
		}
		if segment.Data == nil {
			return fmt.Errorf("消息段 %d 的 data 必须是对象", index)
		}
		if segment.Type == "text" {
			if _, ok := segment.Data["text"].(string); !ok {
				return fmt.Errorf("文本消息段 %d 的 text 必须是字符串", index)
			}
		}
	}
	return nil
}

// ParseOneBot11CQMessage 按 CQ 码规则解析消息，转义后的 [CQ:...] 仍是普通文字。
func ParseOneBot11CQMessage(text string) (OneBot11Message, error) {
	message := OneBot11Message{}
	for text != "" {
		start := strings.Index(text, "[CQ:")
		if start < 0 {
			message = append(message, OneBot11MessageSegment{
				Type: "text", Data: map[string]any{"text": unescapeCQ(text, false)},
			})
			break
		}
		if start > 0 {
			message = append(message, OneBot11MessageSegment{
				Type: "text", Data: map[string]any{"text": unescapeCQ(text[:start], false)},
			})
		}
		end := strings.IndexByte(text[start:], ']')
		if end < 0 {
			return nil, errors.New("CQ 码缺少结束符 ]")
		}
		end += start
		fields := strings.Split(text[start+4:end], ",")
		segment := OneBot11MessageSegment{Type: fields[0], Data: map[string]any{}}
		for _, field := range fields[1:] {
			key, value, found := strings.Cut(field, "=")
			if !found || key == "" || strings.ContainsAny(key, " \t\r\n,=[]") {
				return nil, fmt.Errorf("CQ 码参数 %q 无效", field)
			}
			if _, exists := segment.Data[key]; exists {
				return nil, fmt.Errorf("CQ 码参数 %q 重复", key)
			}
			segment.Data[key] = unescapeCQ(value, true)
		}
		if err := validateOneBot11Segments([]OneBot11MessageSegment{segment}); err != nil {
			return nil, err
		}
		message = append(message, segment)
		text = text[end+1:]
	}
	return message, nil
}

// CQText 输出 CQ 码字符串。含嵌套对象的消息段应使用数组格式。
func (message OneBot11Message) CQText() (string, error) {
	if err := validateOneBot11Segments(message); err != nil {
		return "", err
	}
	var result strings.Builder
	for _, segment := range message {
		if segment.Type == "text" {
			result.WriteString(escapeCQ(segment.Data["text"].(string), false))
			continue
		}
		result.WriteString("[CQ:")
		result.WriteString(segment.Type)
		keys := make([]string, 0, len(segment.Data))
		for key := range segment.Data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if key == "" || strings.ContainsAny(key, " \t\r\n,=[]") {
				return "", fmt.Errorf("CQ 码参数名 %q 无效", key)
			}
			value, err := cqScalar(segment.Data[key])
			if err != nil {
				return "", fmt.Errorf("消息段 %s 的参数 %s：%w", segment.Type, key, err)
			}
			result.WriteByte(',')
			result.WriteString(key)
			result.WriteByte('=')
			result.WriteString(escapeCQ(value, true))
		}
		result.WriteByte(']')
	}
	return result.String(), nil
}

func cqScalar(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(data) == 0 || data[0] == '{' || data[0] == '[' || data[0] == '"' || string(data) == "null" {
		return "", errors.New("CQ 码参数必须是字符串、数字或布尔值；嵌套内容请使用消息段数组")
	}
	return string(data), nil
}

func escapeCQ(text string, parameter bool) string {
	text = strings.NewReplacer("&", "&amp;", "[", "&#91;", "]", "&#93;").Replace(text)
	if parameter {
		text = strings.ReplaceAll(text, ",", "&#44;")
	}
	return text
}

func unescapeCQ(text string, parameter bool) string {
	if parameter {
		text = strings.ReplaceAll(text, "&#44;", ",")
	}
	return strings.NewReplacer("&#91;", "[", "&#93;", "]", "&amp;", "&").Replace(text)
}

var oneBot11MessageFields = []string{
	"time", "self_id", "post_type", "message_type", "sub_type", "message_id", "user_id",
	"group_id", "message", "raw_message", "font", "sender", "anonymous",
}
var oneBot11SenderFields = []string{
	"user_id", "nickname", "sex", "age", "card", "area", "level", "role", "title",
}

func (event *OneBot11MessageEvent) UnmarshalJSON(data []byte) error {
	type eventJSON OneBot11MessageEvent
	var value eventJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	extra, err := remainingJSONFields(data, oneBot11MessageFields)
	if err != nil {
		return err
	}
	value.Extra = extra
	*event = OneBot11MessageEvent(value)
	return nil
}

func (event OneBot11MessageEvent) MarshalJSON() ([]byte, error) {
	type eventJSON OneBot11MessageEvent
	return marshalJSONWithExtra(eventJSON(event), event.Extra, oneBot11MessageFields)
}

func (sender *OneBot11Sender) UnmarshalJSON(data []byte) error {
	type senderJSON OneBot11Sender
	var value senderJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	extra, err := remainingJSONFields(data, oneBot11SenderFields)
	if err != nil {
		return err
	}
	value.Extra = extra
	*sender = OneBot11Sender(value)
	return nil
}

func (sender OneBot11Sender) MarshalJSON() ([]byte, error) {
	type senderJSON OneBot11Sender
	return marshalJSONWithExtra(senderJSON(sender), sender.Extra, oneBot11SenderFields)
}

// 动作参数也使用 json.Number，保证先解码动作再转换 QQ 时 ID 不丢失精度。
func (request *OneBot11ActionRequest) UnmarshalJSON(data []byte) error {
	type requestJSON OneBot11ActionRequest
	var value requestJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*request = OneBot11ActionRequest(value)
	return nil
}

func remainingJSONFields(data []byte, known []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("OneBot 11 事件或发送人必须是 JSON 对象")
	}
	for _, name := range known {
		delete(fields, name)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return fields, nil
}

func marshalJSONWithExtra(value any, extra map[string]json.RawMessage, known []string) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || len(extra) == 0 {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range extra {
		for _, reserved := range known {
			if key == reserved {
				return nil, fmt.Errorf("扩展字段 %q 不能覆盖 OneBot 11 标准字段", key)
			}
		}
		fields[key] = value
	}
	return json.Marshal(fields)
}
