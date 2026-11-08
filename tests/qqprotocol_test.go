package Tests

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// json.Number 防止测试比较时把相邻的大整数误当成相等。
func qqJSONValue(t *testing.T, value any) any {
	t.Helper()
	var data []byte
	var err error
	if raw, ok := value.(string); ok {
		data = []byte(raw)
	} else {
		data, err = json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestQQOneBotModelTextRoundTrip(t *testing.T) {
	const fixture = `{
		"time": 1700000000, "self_id": 9007199254740993, "post_type": "message",
		"message_type": "group", "sub_type": "normal", "message_id": -123,
		"user_id": 12345, "group_id": 54321, "font": 14,
		"raw_message": "原始内容",
		"sender": {"user_id": 12345, "nickname": "Alice", "card": "群名片", "role": "member", "user_uid": "u_abc"},
		"message": [
			{"type": "reply", "data": {"id": "-100"}},
			{"type": "text", "data": {"text": "你好\n字面 [CQ:at,qq=all] & <xml>"}},
			{"type": "at", "data": {"qq": "10001"}},
			{"type": "image", "data": {"file": "photo.jpg", "url": "https://example.test/a?x=1&y=2"}},
			{"type": "custom", "data": {"nested": {"id": 9007199254740995, "flag": true}}},
			{"type": "node", "data": {"user_id": 10001, "nickname": "转发者", "content": [{"type": "text", "data": {"text": "转发正文"}}]}}
		],
		"message_seq": 9007199254740997
	}`
	var event apiImpl.OneBot11MessageEvent
	if err := json.Unmarshal([]byte(fixture), &event); err != nil {
		t.Fatal(err)
	}
	message, err := event.ToQQMessage()
	if err != nil {
		t.Fatal(err)
	}
	if message.SelfID != "9007199254740993" || message.ID != "-123" || len(message.Segments) != 6 {
		t.Fatalf("内部消息丢失 ID 或消息段：%+v", message)
	}
	modelMessage, err := message.ToChatMessage()
	if err != nil {
		t.Fatal(err)
	}
	if modelMessage.Role != "user" || !strings.HasPrefix(modelMessage.Content, apiImpl.QQMessageTextPrefix) {
		t.Fatalf("QQ 消息应转换成 user 角色的纯文本：%+v", modelMessage)
	}
	if modelMessage.Reasoning != "" || len(modelMessage.ToolCalls) != 0 || modelMessage.ToolCallID != "" {
		t.Fatal("QQ 消息不能注入模型的思考或工具字段")
	}
	encoded, err := (&apiImpl.ChatCompletionAPI{}).EncodeRequest(apiImpl.ChatRequest{
		Model: "test-model", Messages: []apiImpl.ChatMessage{modelMessage},
	})
	if err != nil {
		t.Fatal(err)
	}
	var request struct{ Messages []map[string]any }
	if err := json.Unmarshal([]byte(encoded), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Messages) != 1 || len(request.Messages[0]) != 2 || request.Messages[0]["content"] != modelMessage.Content {
		t.Fatalf("模型协议必须只接收 role 和纯文本 content：%s", encoded)
	}
	restoredMessage, err := apiImpl.QQMessageFromChatMessage(modelMessage)
	if err != nil {
		t.Fatal(err)
	}
	restoredEvent, err := restoredMessage.ToOneBot11MessageEvent()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := qqJSONValue(t, restoredEvent), qqJSONValue(t, fixture); !reflect.DeepEqual(got, want) {
		t.Fatalf("OneBot→QQ→模型文本→QQ→OneBot 丢失信息：\ngot: %#v\nwant: %#v", got, want)
	}
	// 转换后的内部消息应有独立数据，修改不会影响原始事件。
	message.Segments[1].Data["text"] = "修改后的正文"
	if event.Message[1].Data["text"] == "修改后的正文" {
		t.Fatal("QQ 和 OneBot 消息段共享了可变数据")
	}
}

func TestQQPrivateAndSelfMessages(t *testing.T) {
	for _, postType := range []string{apiImpl.OneBot11PostMessage, apiImpl.OneBot11PostMessageSent} {
		t.Run(postType, func(t *testing.T) {
			event := apiImpl.OneBot11MessageEvent{
				OneBot11EventHeader: apiImpl.OneBot11EventHeader{Time: 123, SelfID: 10001, PostType: postType},
				MessageType:         "private", SubType: "friend", UserID: 10002, MessageID: 100,
				Message: apiImpl.OneBot11Message{{Type: "text", Data: map[string]any{"text": "私聊正文"}}},
				Sender:  &apiImpl.OneBot11Sender{UserID: 10002, Nickname: "Bob"},
			}
			message, err := event.ToQQMessage()
			if err != nil {
				t.Fatal(err)
			}
			model, err := message.ToChatMessage()
			if err != nil {
				t.Fatal(err)
			}
			wantRole := "user"
			if postType == apiImpl.OneBot11PostMessageSent {
				wantRole = "assistant"
			}
			if model.Role != wantRole {
				t.Fatalf("发送方向错误：%s", model.Role)
			}
			back, err := apiImpl.QQMessageFromChatMessage(model)
			if err != nil {
				t.Fatal(err)
			}
			got, err := back.ToOneBot11MessageEvent()
			if err != nil || !reflect.DeepEqual(qqJSONValue(t, got), qqJSONValue(t, event)) {
				t.Fatalf("私聊或自身消息往返失败：%+v，错误=%v", got, err)
			}
		})
	}
}

func TestQQPlainModelReplyAndSendAction(t *testing.T) {
	const body = "普通模型回复\n[CQ:at,qq=all] & [图片]"
	message, err := apiImpl.QQMessageFromChatMessage(apiImpl.ChatMessage{
		Role: "assistant", Content: body, Reasoning: "不发送的思考",
		ToolCalls: []apiImpl.ToolCall{{ID: "call", Name: "ReadFile", Arguments: "{}"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Segments) != 1 || message.Segments[0].Type != apiImpl.QQSegmentText || message.Segments[0].Data["text"] != body {
		t.Fatalf("普通回复应完整保留为一个文本段：%+v", message)
	}
	message.Type, message.GroupID = apiImpl.QQMessageGroup, "9007199254740993"
	echo := json.RawMessage(`"send-1"`)
	action, err := message.ToOneBot11Action(echo)
	if err != nil {
		t.Fatal(err)
	}
	echo[1] = 'X'
	if string(action.Echo) != `"send-1"` {
		t.Fatal("动作 echo 不应与调用方共享缓冲区")
	}
	if action.Action != "send_group_msg" || action.Params["group_id"].(apiImpl.OneBot11ID).String() != message.GroupID {
		t.Fatalf("群消息发送动作错误：%+v", action)
	}
	data, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	var decoded apiImpl.OneBot11ActionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := apiImpl.QQMessageFromOneBot11Action(decoded)
	if err != nil || restored.GroupID != message.GroupID || restored.Segments[0].Data["text"] != body {
		t.Fatalf("发送动作往返丢失正文或整数精度：%+v，错误=%v", restored, err)
	}
	if _, err := message.ToOneBot11MessageEvent(); err == nil {
		t.Fatal("待发送消息不能伪造为已存在的消息事件")
	}
}

func TestOneBot11CQCodec(t *testing.T) {
	const fixture = `A&amp;&#91;CQ:at,qq=all&#93;[CQ:at,qq=all][CQ:image,file=a&#44;b&#91;c&#93;&amp;.jpg]尾&amp;#91;`
	message, err := apiImpl.ParseOneBot11CQMessage(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(message) != 4 || message[0].Data["text"] != "A&[CQ:at,qq=all]" || message[2].Data["file"] != "a,b[c]&.jpg" || message[3].Data["text"] != "尾&#91;" {
		t.Fatalf("CQ 转义或消息顺序错误：%+v", message)
	}
	text, err := message.CQText()
	if err != nil || text != fixture {
		t.Fatalf("CQ 往返错误：%q，错误=%v", text, err)
	}
	jsonString, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var fromString apiImpl.OneBot11Message
	if err := json.Unmarshal(jsonString, &fromString); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromString, message) {
		t.Fatal("OneBot 字符串消息没有按 CQ 码解析")
	}
	encoded, err := json.Marshal(fromString)
	if err != nil || len(encoded) == 0 || encoded[0] != '[' {
		t.Fatalf("消息编码应统一使用数组格式：%s，错误=%v", encoded, err)
	}
	nested := apiImpl.OneBot11Message{{Type: "node", Data: map[string]any{"content": []any{map[string]any{"type": "text", "data": map[string]any{"text": "嵌套"}}}}}}
	if _, err := nested.CQText(); err == nil {
		t.Fatal("嵌套消息不能悄悄转换成不支持的 CQ 参数")
	}
	for _, invalid := range []string{"[CQ:at", "[CQ:]", "[CQ:at,bad]", "[CQ:at,qq=1,qq=2]", "[CQ:at,=1]"} {
		if _, err := apiImpl.ParseOneBot11CQMessage(invalid); err == nil {
			t.Fatalf("应拒绝无效 CQ 码：%s", invalid)
		}
	}
}

func TestOneBot11SendActionFormats(t *testing.T) {
	for _, tc := range []struct {
		name      string
		action    string
		params    map[string]any
		wantType  apiImpl.QQMessageType
		wantID    string
		wantCount int
	}{
		{name: "CQ string", action: "send_private_msg", params: map[string]any{"user_id": "12345", "message": "你好[CQ:at,qq=all]"}, wantType: apiImpl.QQMessagePrivate, wantID: "12345", wantCount: 2},
		{name: "auto escape", action: "send_private_msg", params: map[string]any{"user_id": 12345, "message": "[CQ:at,qq=all]", "auto_escape": true}, wantType: apiImpl.QQMessagePrivate, wantID: "12345", wantCount: 1},
		{name: "infer group", action: "send_msg", params: map[string]any{"group_id": 54321, "message": "群消息"}, wantType: apiImpl.QQMessageGroup, wantID: "54321", wantCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message, err := apiImpl.QQMessageFromOneBot11Action(apiImpl.OneBot11ActionRequest{Action: tc.action, Params: tc.params})
			if err != nil || message.Type != tc.wantType || len(message.Segments) != tc.wantCount {
				t.Fatalf("发送动作解析错误：%+v，错误=%v", message, err)
			}
			if message.UserID != tc.wantID && message.GroupID != tc.wantID {
				t.Fatalf("发送对象错误：%+v", message)
			}
			if tc.name == "auto escape" && message.Segments[0].Data["text"] != "[CQ:at,qq=all]" {
				t.Fatal("auto_escape 应保留 CQ 字样为纯文本")
			}
		})
	}
}

func TestQQProtocolRejectsInvalidData(t *testing.T) {
	for _, data := range []string{`null`, `1.5`, `"not-id"`, `9223372036854775808`} {
		var id apiImpl.OneBot11ID
		if err := json.Unmarshal([]byte(data), &id); err == nil {
			t.Fatalf("应拒绝无效 ID：%s", data)
		}
	}
	for _, data := range []string{
		`{}`, `123`, `[{"type":"text","data":{"text":123}}]`,
		`[{"type":"","data":{}}]`, `[{"type":"at","data":null}]`,
	} {
		var message apiImpl.OneBot11Message
		if err := json.Unmarshal([]byte(data), &message); err == nil {
			t.Fatalf("应拒绝无效 OneBot 消息：%s", data)
		}
	}
	for _, model := range []apiImpl.ChatMessage{
		{Role: "system", Content: "系统指令"},
		{Role: "tool", Content: "工具结果"},
		{Role: "assistant"},
		{Role: "user", Content: apiImpl.QQMessageTextPrefix + "{}"},
		{Role: "user", Content: apiImpl.QQMessageTextPrefix + `{"sent_by_self":true,"segments":[{"type":"text","data":{"text":"x"}}]}`},
		{Role: "user", Content: apiImpl.QQMessageTextPrefix + `{"segments":[{"type":"text","data":{"text":"x"}}]}{}`},
	} {
		if _, err := apiImpl.QQMessageFromChatMessage(model); err == nil {
			t.Fatalf("应拒绝无效 QQ 模型消息：%+v", model)
		}
	}
	base := apiImpl.QQMessage{Type: apiImpl.QQMessageGroup, GroupID: "54321", Segments: []apiImpl.QQMessageSegment{apiImpl.QQText("正文")}}
	for _, groupID := range []string{"", "0", "-1", "abc", "9223372036854775808"} {
		message := base
		message.GroupID = groupID
		if _, err := message.ToOneBot11Action(nil); err == nil {
			t.Fatalf("应拒绝无效群号：%s", groupID)
		}
	}
	if _, err := base.ToOneBot11Action(json.RawMessage("bad")); err == nil {
		t.Fatal("应拒绝无效 echo JSON")
	}
	if _, err := (apiImpl.OneBot11MessageEvent{OneBot11EventHeader: apiImpl.OneBot11EventHeader{PostType: "notice"}}).ToQQMessage(); err == nil {
		t.Fatal("通知不能当作消息事件转换")
	}
}
