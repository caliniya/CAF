package Tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	onebot11qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func useReverseWS(t *testing.T, config onebot11qq.ReverseWSConfig) (*onebot11qq.ReverseWSClient, string) {
	t.Helper()
	client, err := onebot11qq.NewReverseWSClient(config)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(client)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
		server.Close()
	})
	path := config.Path
	if path == "" {
		path = "/onebot/v11/ws"
	}
	return client, "ws" + strings.TrimPrefix(server.URL, "http") + path
}

func connectReverseWS(t *testing.T, address, selfID, token string) *websocket.Conn {
	t.Helper()
	headers := http.Header{"X-Self-ID": {selfID}, "X-Client-Role": {"Universal"}}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	connection, response, err := websocket.DefaultDialer.Dial(address, headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func nextQQEvent(t *testing.T, events <-chan onebot11qq.Event, eventType onebot11qq.EventType) onebot11qq.Event {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				t.Fatal("QQ 事件流提前关闭")
			}
			if event.Type == onebot11qq.EventError {
				t.Fatalf("非预期的 QQ 协议错误：%s", event.Error)
			}
			if event.Type == eventType {
				return event
			}
		case <-timer.C:
			t.Fatalf("等待 QQ %s 事件超时", eventType)
		}
	}
}

func writeOneBotPacket(t *testing.T, connection *websocket.Conn, value any) {
	t.Helper()
	if err := connection.WriteJSON(value); err != nil {
		t.Fatal(err)
	}
}

func readOneBotAction(t *testing.T, connection *websocket.Conn) onebot11qq.OneBot11ActionRequest {
	t.Helper()
	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var action onebot11qq.OneBot11ActionRequest
	if err := connection.ReadJSON(&action); err != nil {
		t.Fatal(err)
	}
	return action
}

type qqSendOutcome struct {
	result onebot11qq.SendResult
	err    error
}

func sendQQAsync(client onebot11qq.QQClient, ctx context.Context, message onebot11qq.QQMessage) <-chan qqSendOutcome {
	result := make(chan qqSendOutcome, 1)
	go func() {
		sent, err := client.SendMessage(ctx, message)
		result <- qqSendOutcome{result: sent, err: err}
	}()
	return result
}

func qqTestGroupMessage(text string) onebot11qq.QQMessage {
	return onebot11qq.QQMessage{
		Type: onebot11qq.QQMessageGroup, GroupID: "23456",
		Segments: []onebot11qq.QQMessageSegment{onebot11qq.QQText(text)},
	}
}

func waitQQSend(t *testing.T, result <-chan qqSendOutcome) qqSendOutcome {
	t.Helper()
	select {
	case sent := <-result:
		return sent
	case <-time.After(3 * time.Second):
		t.Fatal("QQ 发送调用没有及时结束")
		return qqSendOutcome{}
	}
}

func TestQQReverseWSReceivesMessagesAndRequests(t *testing.T) {
	client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{AccessToken: "token"})
	connection := connectReverseWS(t, address, "10001", "token")
	status := nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if status.Info == nil || !status.Info.Connected || status.Info.SelfID != "10001" || status.Info.Online != nil {
		t.Fatalf("连接状态错误：%+v", status)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 123, "self_id": 10001, "post_type": "meta_event",
		"meta_event_type": "heartbeat", "interval": 5000, "status": map[string]bool{"online": true},
	})
	status = nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if status.Info.Online == nil || !*status.Info.Online {
		t.Fatalf("没有识别 QQ 在线状态：%+v", status)
	}
	snapshot := client.Info()
	*snapshot.Online = false
	if !*client.Info().Online {
		t.Fatal("Info 返回值共享了可变状态")
	}
	for _, messageType := range []string{"private", "group"} {
		writeOneBotPacket(t, connection, map[string]any{
			"time": 124, "self_id": 10001, "post_type": "message",
			"message_type": messageType, "message_id": -101, "user_id": 12345, "group_id": 23456,
			"message": "你好[CQ:at,qq=10001]", "sender": map[string]any{"user_id": 12345, "nickname": "Alice"},
		})
		event := nextQQEvent(t, client.Events(), onebot11qq.EventMessage)
		if event.Message == nil || string(event.Message.Type) != messageType || event.Message.ID != "-101" ||
			event.Message.SentBySelf || len(event.Message.Segments) != 2 || event.Message.Segments[1].Type != onebot11qq.QQSegmentAt {
			t.Fatalf("消息没有转换成内部 QQ 协议：%+v", event.Message)
		}
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 125, "self_id": 10001, "post_type": "message_sent",
		"message_type": "group", "message_id": 102, "user_id": 10001, "group_id": 23456,
		"message": []any{map[string]any{"type": "text", "data": map[string]string{"text": "自身消息"}}},
		"sender":  map[string]any{"user_id": 10001, "nickname": "CAF"},
	})
	event := nextQQEvent(t, client.Events(), onebot11qq.EventMessage)
	if event.Message == nil || !event.Message.SentBySelf || client.Info().Nickname != "CAF" {
		t.Fatalf("自身消息或昵称识别错误：%+v", event)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 125, "self_id": 10001, "post_type": "message",
		"message_type": "group", "message_id": 103, "user_id": 10001, "group_id": 23456,
		"message": "作为普通事件上报的自身消息",
	})
	event = nextQQEvent(t, client.Events(), onebot11qq.EventMessage)
	if event.Message == nil || !event.Message.SentBySelf {
		t.Fatalf("应通过账号识别普通 message 中的自身消息：%+v", event)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 126, "self_id": 10001, "post_type": "request", "request_type": "friend",
		"user_id": 12345, "comment": "你好，请加好友", "flag": "friend-flag",
	})
	event = nextQQEvent(t, client.Events(), onebot11qq.EventRequest)
	if event.Request == nil || event.Request.Type != "friend" || event.Request.UserID != "12345" ||
		event.Request.Flag != "friend-flag" || event.Request.Comment != "你好，请加好友" {
		t.Fatalf("好友申请没有完整上报：%+v", event)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 127, "self_id": 10001, "post_type": "notice", "notice_type": "group_recall",
		"group_id": 23456, "user_id": 12345, "operator_id": 12345, "message_id": -101,
	})
	event = nextQQEvent(t, client.Events(), onebot11qq.EventNotice)
	if event.Notice == nil || event.Notice.Type != "group_recall" || event.Notice.MessageID != "-101" || len(event.Raw) == 0 {
		t.Fatalf("撤回通知没有完整上报：%+v", event)
	}
}

func TestQQReverseWSAuthenticatesAndBindsAccount(t *testing.T) {
	client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{AccessToken: "token", SelfID: "10001"})
	for _, tc := range []struct {
		name    string
		address string
		headers http.Header
		code    int
	}{
		{name: "missing token", address: address, headers: http.Header{"X-Self-ID": {"10001"}}, code: http.StatusUnauthorized},
		{name: "bad token", address: address, headers: http.Header{"X-Self-ID": {"10001"}, "Authorization": {"Bearer bad"}}, code: http.StatusUnauthorized},
		{name: "missing account", address: address, headers: http.Header{"Authorization": {"Bearer token"}}, code: http.StatusBadRequest},
		{name: "other account", address: address, headers: http.Header{"X-Self-ID": {"10002"}, "Authorization": {"Bearer token"}}, code: http.StatusConflict},
		{name: "separate event connection", address: address, headers: http.Header{"X-Self-ID": {"10001"}, "Authorization": {"Bearer token"}, "X-Client-Role": {"Event"}}, code: http.StatusBadRequest},
		{name: "wrong path", address: address + "/wrong", headers: http.Header{"X-Self-ID": {"10001"}, "Authorization": {"Bearer token"}}, code: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connection, response, err := websocket.DefaultDialer.Dial(tc.address, tc.headers)
			if connection != nil {
				_ = connection.Close()
			}
			if response != nil {
				defer response.Body.Close()
			}
			if err == nil || response == nil || response.StatusCode != tc.code {
				t.Fatalf("应拒绝连接，status=%v，错误=%v", response, err)
			}
		})
	}
	connection := connectReverseWS(t, address+"?access_token=token", "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if !client.Info().Connected {
		t.Fatal("查询参数 token 应允许连接")
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 1, "self_id": 10002, "post_type": "message", "message_type": "private",
		"message_id": 1, "user_id": 12345, "message": "别的账号",
	})
	event := nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if event.Info == nil || event.Info.Connected || !strings.Contains(event.Info.LastError, "账号") {
		t.Fatalf("应断开账号不匹配的连接：%+v", event)
	}
}

func TestQQReverseWSSendResponseCorrelation(t *testing.T) {
	client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{RequestTimeout: time.Second})
	connection := connectReverseWS(t, address, "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	first := sendQQAsync(client, context.Background(), qqTestGroupMessage("first"))
	firstAction := readOneBotAction(t, connection)
	second := sendQQAsync(client, context.Background(), onebot11qq.QQMessage{
		Type: onebot11qq.QQMessagePrivate, UserID: "12345",
		Segments: []onebot11qq.QQMessageSegment{onebot11qq.QQText("second[CQ:at,qq=all]")},
	})
	secondAction := readOneBotAction(t, connection)
	if firstAction.Action != "send_group_msg" || secondAction.Action != "send_private_msg" ||
		string(firstAction.Echo) == string(secondAction.Echo) {
		t.Fatalf("动作名称或 echo 错误：%+v，%+v", firstAction, secondAction)
	}
	secondMessage, err := onebot11qq.QQMessageFromOneBot11Action(secondAction)
	if err != nil || len(secondMessage.Segments) != 1 || secondMessage.Segments[0].Data["text"] != "second[CQ:at,qq=all]" {
		t.Fatalf("纯文本回复不能误解析为 CQ 消息：%+v，错误=%v", secondMessage, err)
	}
	// 对端逆序返回，验证响应依赖 echo 而不是发送顺序。
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", RetCode: 0, Echo: secondAction.Echo, Data: json.RawMessage(`{"message_id":-202}`),
	})
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", RetCode: 0, Echo: firstAction.Echo, Data: json.RawMessage(`{"message_id":"201"}`),
	})
	if result := waitQQSend(t, first); result.err != nil || result.result.MessageID != "201" {
		t.Fatalf("第一条消息响应匹配错误：%+v", result)
	}
	if result := waitQQSend(t, second); result.err != nil || result.result.MessageID != "-202" {
		t.Fatalf("第二条消息响应匹配错误：%+v", result)
	}
}

func TestQQReverseWSSendErrorsAndCancellation(t *testing.T) {
	client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{RequestTimeout: 150 * time.Millisecond})
	if _, err := client.SendMessage(context.Background(), qqTestGroupMessage("offline")); !errors.Is(err, onebot11qq.ErrNotConnected) {
		t.Fatalf("未连接应返回明确错误：%v", err)
	}
	connection := connectReverseWS(t, address, "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := sendQQAsync(client, ctx, qqTestGroupMessage("cancelled"))
	cancelledAction := readOneBotAction(t, connection)
	cancel()
	result := waitQQSend(t, cancelled)
	var sendError *onebot11qq.SendError
	if !errors.Is(result.err, context.Canceled) || !errors.As(result.err, &sendError) || !sendError.MayHaveSent {
		t.Fatalf("请求已发出后的取消必须标记发送结果未知：%v", result.err)
	}
	// 迟到响应不应该污染后续请求或断开连接。
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", Echo: cancelledAction.Echo, Data: json.RawMessage(`{"message_id":1}`),
	})
	failed := sendQQAsync(client, context.Background(), qqTestGroupMessage("failed"))
	failedAction := readOneBotAction(t, connection)
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "failed", RetCode: 100, Message: "发送被拒绝", Echo: failedAction.Echo, Data: json.RawMessage("null"),
	})
	result = waitQQSend(t, failed)
	var actionError *onebot11qq.ActionError
	if !errors.As(result.err, &actionError) || !errors.As(result.err, &sendError) || sendError.MayHaveSent || actionError.RetCode != 100 {
		t.Fatalf("明确失败响应错误：%v", result.err)
	}
	timedOut := sendQQAsync(client, context.Background(), qqTestGroupMessage("timeout"))
	readOneBotAction(t, connection)
	result = waitQQSend(t, timedOut)
	if !errors.Is(result.err, context.DeadlineExceeded) || !errors.As(result.err, &sendError) || !sendError.MayHaveSent || !client.Info().Connected {
		t.Fatalf("等待响应超时应保持连接并标记结果未知：%v", result.err)
	}
	missingID := sendQQAsync(client, context.Background(), qqTestGroupMessage("missing ID"))
	action := readOneBotAction(t, connection)
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", Echo: action.Echo, Data: json.RawMessage("{}"),
	})
	result = waitQQSend(t, missingID)
	if result.err == nil || !errors.As(result.err, &sendError) || !sendError.MayHaveSent {
		t.Fatalf("成功响应缺少消息 ID 应报告结果未知：%v", result.err)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"time": 1, "self_id": 10001, "post_type": "meta_event", "meta_event_type": "heartbeat",
		"status": map[string]bool{"online": false},
	})
	status := nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if status.Info.Online == nil || *status.Info.Online {
		t.Fatal("应识别 QQ 离线")
	}
	if _, err := client.SendMessage(context.Background(), qqTestGroupMessage("offline")); !errors.Is(err, onebot11qq.ErrQQOffline) {
		t.Fatalf("QQ 离线不应尝试发送：%v", err)
	}
}

func TestQQReverseWSReconnectAndClose(t *testing.T) {
	client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{})
	firstConnection := connectReverseWS(t, address, "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	pending := sendQQAsync(client, context.Background(), qqTestGroupMessage("old connection"))
	readOneBotAction(t, firstConnection)
	secondConnection := connectReverseWS(t, address, "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	outcome := waitQQSend(t, pending)
	var sendError *onebot11qq.SendError
	if outcome.err == nil || !errors.As(outcome.err, &sendError) || !sendError.MayHaveSent {
		t.Fatalf("替换旧连接必须结束旧请求：%+v", outcome)
	}
	current := sendQQAsync(client, context.Background(), qqTestGroupMessage("new connection"))
	action := readOneBotAction(t, secondConnection)
	writeOneBotPacket(t, secondConnection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", Echo: action.Echo, Data: json.RawMessage(`{"message_id":303}`),
	})
	if outcome := waitQQSend(t, current); outcome.err != nil || outcome.result.MessageID != "303" {
		t.Fatalf("重连后发送失败：%+v", outcome)
	}
	if err := secondConnection.Close(); err != nil {
		t.Fatal(err)
	}
	status := nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if status.Info.Connected || client.Info().Connected {
		t.Fatal("远端断开后应更新连接状态")
	}
	connection := connectReverseWS(t, address, "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	closing := sendQQAsync(client, context.Background(), qqTestGroupMessage("closing"))
	readOneBotAction(t, connection)
	var closers sync.WaitGroup
	for range 4 {
		closers.Go(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	closers.Wait()
	if outcome := waitQQSend(t, closing); !errors.Is(outcome.err, onebot11qq.ErrClientClosed) {
		t.Fatalf("关闭必须结束等待请求：%v", outcome.err)
	}
	select {
	case _, open := <-client.Events():
		if open {
			t.Fatal("关闭后事件流应结束")
		}
	case <-time.After(time.Second):
		t.Fatal("关闭后事件流没有结束")
	}
}

func TestQQReverseWSEventQueueDoesNotBlockResponses(t *testing.T) {
	client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{EventBuffer: 2})
	connection := connectReverseWS(t, address, "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	// 故意不读取消息事件，动作响应仍必须能完成。
	writeOneBotPacket(t, connection, map[string]any{
		"time": 1, "self_id": 10001, "post_type": "message", "message_type": "private",
		"message_id": 1, "user_id": 12345, "message": "队列中的消息",
	})
	pending := sendQQAsync(client, context.Background(), qqTestGroupMessage("reply"))
	action := readOneBotAction(t, connection)
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", Echo: action.Echo, Data: json.RawMessage(`{"message_id":4}`),
	})
	if outcome := waitQQSend(t, pending); outcome.err != nil {
		t.Fatalf("事件消费者不能阻塞动作响应：%v", outcome.err)
	}
	for id := 2; id < 10; id++ {
		if err := connection.WriteJSON(map[string]any{
			"time": 1, "self_id": 10001, "post_type": "message", "message_type": "private",
			"message_id": id, "user_id": 12345, "message": fmt.Sprint(id),
		}); err != nil {
			break
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for client.Info().Connected && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if client.Info().Connected || !strings.Contains(client.Info().LastError, "队列") {
		t.Fatalf("事件队列溢出应明确断开而不是静默丢消息：%+v", client.Info())
	}
	status := nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	if status.Info == nil || status.Info.Connected || !strings.Contains(status.Info.LastError, "队列") {
		t.Fatalf("队列满后应保留断开状态供上层读取：%+v", status)
	}
}

func TestQQReverseWSStartAndContextClose(t *testing.T) {
	client, err := onebot11qq.NewReverseWSClient(onebot11qq.ReverseWSConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err == nil {
		t.Fatal("应拒绝重复启动")
	}
	connectReverseWS(t, "ws://"+client.Address()+"/onebot/v11/ws", "10001", "")
	nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
	cancel()
	select {
	case _, open := <-client.Events():
		if open {
			t.Fatal("取消运行 context 后事件流应关闭")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消 context 后服务没有结束")
	}
}

func TestQQReverseWSPingAndReadTimeout(t *testing.T) {
	for _, responsive := range []bool{true, false} {
		t.Run(fmt.Sprint(responsive), func(t *testing.T) {
			client, address := useReverseWS(t, onebot11qq.ReverseWSConfig{
				PingInterval: 20 * time.Millisecond, PongTimeout: 150 * time.Millisecond,
			})
			connection := connectReverseWS(t, address, "10001", "")
			nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
			if responsive {
				pingReceived := make(chan struct{}, 1)
				connection.SetPingHandler(func(payload string) error {
					select {
					case pingReceived <- struct{}{}:
					default:
					}
					return connection.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second))
				})
				readFinished := make(chan struct{})
				go func() {
					defer close(readFinished)
					for {
						if _, _, err := connection.ReadMessage(); err != nil {
							return
						}
					}
				}()
				for range 10 {
					select {
					case <-pingReceived:
					case <-time.After(time.Second):
						t.Fatal("没有收到反向 WS 心跳")
					}
				}
				if !client.Info().Connected {
					t.Fatal("正常响应心跳的连接不应因空闲断开")
				}
				_ = connection.Close()
				select {
				case <-readFinished:
				case <-time.After(time.Second):
					t.Fatal("模拟客户端读取没有退出")
				}
			} else {
				event := nextQQEvent(t, client.Events(), onebot11qq.EventStatus)
				if event.Info == nil || event.Info.Connected || event.Info.LastError == "" {
					t.Fatalf("没有响应心跳的连接应超时断开：%+v", event)
				}
			}
		})
	}
}

func TestQQReverseWSRejectsMalformedConfig(t *testing.T) {
	for _, config := range []onebot11qq.ReverseWSConfig{
		{Path: "not-absolute"},
		{Path: "/ws?token=anything"},
		{SelfID: "-1"},
		{RequestTimeout: -time.Second},
		{EventBuffer: -1},
		{PingInterval: time.Second, PongTimeout: time.Second},
	} {
		if client, err := onebot11qq.NewReverseWSClient(config); err == nil {
			_ = client.Close()
			t.Fatalf("应拒绝无效配置：%+v", config)
		}
	}
}
