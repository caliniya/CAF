package Tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	qq "github.com/caliniya/caf/core/Extension/QQ"
	"github.com/gorilla/websocket"
)

func TestQQCommandsUseNamedPrefixes(t *testing.T) {
	for _, text := range []string{"hi", "/caf status", "hello /助手 help", "/助手2 help", "/助手help"} {
		if _, ok := qq.ParseCommand(qq.QQMessage{Segments: []qq.QQMessageSegment{qq.QQText(text)}}, "助手"); ok {
			t.Fatalf("recognized ordinary/wrong-prefix message %q", text)
		}
	}
	message := qq.QQMessage{SelfID: "10001", Segments: []qq.QQMessageSegment{
		{Type: qq.QQSegmentReply, Data: map[string]any{"id": "1"}},
		{Type: qq.QQSegmentAt, Data: map[string]any{"qq": "10001"}},
		qq.QQText(" /助手 status "),
	}}
	command, ok := qq.ParseCommand(message, "助手")
	if !ok || command.Name != "status" || len(command.Args) != 0 {
		t.Fatalf("could not parse bot mention command: %+v", command)
	}
	message.SentBySelf = true
	if _, ok := qq.ParseCommand(message, "助手"); ok {
		t.Fatal("self message triggered command")
	}
	message.SentBySelf = false
	message.Segments[1].Data["qq"] = "20002"
	if _, ok := qq.ParseCommand(message, "助手"); ok {
		t.Fatal("mention of other account triggered command")
	}
	message.Segments = []qq.QQMessageSegment{qq.QQText("/助手 help"), {Type: qq.QQSegmentImage, Data: map[string]any{"file": "x"}}}
	if _, ok := qq.ParseCommand(message, "助手"); ok {
		t.Fatal("image command accepted")
	}
	command, ok = qq.ParseCommand(qq.QQMessage{Segments: []qq.QQMessageSegment{qq.QQText("/助手")}}, "助手")
	if !ok || command.Name != "help" {
		t.Fatal("bare prefix should return help")
	}
}

func TestQQCommandsReplyOnlyForEnabledNamedExtension(t *testing.T) {
	instance := useQQAdapterCore(t)
	owner, err := instance.CreateExtensionClient("Main", "main")
	if err != nil {
		t.Fatal(err)
	}
	first, err := instance.CreateQQAdapter("QQ1", "token", "10001")
	if err != nil {
		t.Fatal(err)
	}
	second, err := instance.CreateQQAdapter("QQ2", "token", "20002")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, name string }{{first.ID, "助手"}, {second.ID, "helper"}} {
		if _, err := instance.SetExtensionClientQQ(owner.ID, item.id, item.name, true); err != nil {
			t.Fatal(err)
		}
	}
	connection := connectReverseWS(t, first.Address, "10001", "token")
	connection2 := connectReverseWS(t, second.Address, "20002", "token")
	writeMessage := func(conn *websocket.Conn, selfID, messageID int, text string) {
		writeOneBotPacket(t, conn, map[string]any{"post_type": "message", "message_type": "group", "sub_type": "normal", "self_id": selfID, "message_id": messageID, "group_id": 23456, "user_id": 30003, "time": 123, "message": []map[string]any{{"type": "text", "data": map[string]any{"text": text}}}})
	}
	writeMessage(connection, 10001, 1, "你好")
	writeMessage(connection, 10001, 2, "/helper status")
	writeMessage(connection, 10001, 3, "/助手 status")
	action := readOneBotAction(t, connection)
	response, err := qq.QQMessageFromOneBot11Action(action)
	if err != nil {
		t.Fatal(err)
	}
	text := response.Segments[0].Data["text"].(string)
	if !strings.Contains(text, "扩展：助手") || !strings.Contains(text, "ID：main") || response.GroupID != "23456" {
		t.Fatalf("wrong command reply: %+v", response)
	}
	writeOneBotPacket(t, connection, qq.OneBot11ActionResponse{Status: "ok", Echo: action.Echo, Data: json.RawMessage(`{"message_id":101}`)})
	writeMessage(connection2, 20002, 4, "/helper help")
	action2 := readOneBotAction(t, connection2)
	response, err = qq.QQMessageFromOneBot11Action(action2)
	if err != nil || !strings.Contains(response.Segments[0].Data["text"].(string), "/helper status") {
		t.Fatalf("second extension prefix not used: %+v / %v", response, err)
	}
	writeOneBotPacket(t, connection2, qq.OneBot11ActionResponse{Status: "ok", Echo: action2.Echo, Data: json.RawMessage(`{"message_id":102}`)})
	writeMessage(connection, 10001, 3, "/助手 status") // 已处理的重复事件
	writeMessage(connection, 10001, 5, "/助手 help")
	next := readOneBotAction(t, connection)
	response, err = qq.QQMessageFromOneBot11Action(next)
	if err != nil || !strings.Contains(response.Segments[0].Data["text"].(string), "可用指令") {
		t.Fatal("duplicate command was replied to again")
	}
	writeOneBotPacket(t, connection, qq.OneBot11ActionResponse{Status: "ok", Echo: next.Echo, Data: json.RawMessage(`{"message_id":103}`)})
	if _, err := instance.SetExtensionClientQQ(owner.ID, first.ID, "助手", false); err != nil {
		t.Fatal(err)
	}
	waitQQAdapter(t, instance, first.ID, "stopped", "10001")
	waitQQAdapter(t, instance, second.ID, "connected", "20002")
	if _, err := instance.StartQQAdapter(first.ID); err != nil {
		t.Fatal(err)
	}
	connection = connectReverseWS(t, first.Address, "10001", "token")
	writeMessage(connection, 10001, 6, "/助手 status")
	_ = connection.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("disabled extension replied to command")
	}
}
