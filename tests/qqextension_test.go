package Tests

import (
	"context"
	"encoding/json"
	"errors"
	extension "github.com/caliniya/caf/core/Extension"
	onebot11qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	"net"
	"sync"
	"testing"
	"time"
)

func useQQExtension(t *testing.T, config onebot11qq.ReverseWSConfig) (*onebot11qq.ReverseWSExtension, string) {
	t.Helper()
	config.Address = "127.0.0.1:0"
	instance, err := onebot11qq.NewReverseWSExtension(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	if err := instance.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 端口由系统分配，监听地址属于实现专用信息。
	return instance, "ws://" + instance.Address() + "/onebot/v11/ws"
}

func nextExtensionEvent(t *testing.T, instance extension.Extension, eventType string) extension.Event {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, open := <-instance.Events():
			if !open {
				t.Fatal("extension event stream closed early")
			}
			if event.Type == eventType {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", eventType)
		}
	}
}

func TestQQExtensionDescriptorAndLifecycle(t *testing.T) {
	instance, err := onebot11qq.NewReverseWSExtension(onebot11qq.ReverseWSConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	var generic extension.Extension = instance
	descriptor := generic.Describe()
	if descriptor.Type != onebot11qq.ExtensionType || descriptor.Implementation != "onebot11-reverse-ws" ||
		len(descriptor.Capabilities) != 1 || descriptor.Capabilities[0].Name != onebot11qq.CapabilitySendMessage ||
		!json.Valid(descriptor.Capabilities[0].Parameters) {
		t.Fatalf("invalid extension descriptor: %+v", descriptor)
	}
	descriptor.Capabilities[0].Name = "changed"
	descriptor.Capabilities[0].Parameters[0] = 'x'
	if current := generic.Describe(); current.Capabilities[0].Name != onebot11qq.CapabilitySendMessage || !json.Valid(current.Capabilities[0].Parameters) {
		t.Fatal("descriptor snapshot changed extension behavior")
	}
	if generic.Status().State != extension.StateStopped {
		t.Fatal("new extension should be stopped")
	}
	arguments, _ := json.Marshal(qqTestGroupMessage("test"))
	if _, err := generic.Invoke(context.Background(), onebot11qq.CapabilitySendMessage, arguments); !errors.Is(err, extension.ErrNotStarted) {
		t.Fatalf("invoked stopped extension: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := generic.Start(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("accepted canceled startup: %v", err)
	}
	if err := generic.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := generic.Start(context.Background()); !errors.Is(err, extension.ErrAlreadyStarted) {
		t.Fatalf("unexpected repeated start result: %v", err)
	}
	status := generic.Status()
	if status.State != extension.StateRunning || status.Connected == nil || *status.Connected {
		t.Fatalf("listener waiting for connection has wrong status: %+v", status)
	}
	*status.Connected = true
	if *generic.Status().Connected {
		t.Fatal("status snapshot changed connected state")
	}
	var closers sync.WaitGroup
	for range 4 {
		closers.Go(func() {
			if err := generic.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	closers.Wait()
	if generic.Status().State != extension.StateClosed {
		t.Fatal("closed extension state not reported")
	}
	if _, open := <-generic.Events(); open {
		t.Fatal("Close did not end event stream")
	}
	if err := generic.Start(context.Background()); !errors.Is(err, extension.ErrClosed) {
		t.Fatalf("restarted closed extension: %v", err)
	}
}

func TestQQExtensionPreservesDomainEventsAndRouting(t *testing.T) {
	instance, address := useQQExtension(t, onebot11qq.ReverseWSConfig{AccessToken: "token"})
	connection := connectReverseWS(t, address, "10001", "token")
	nextExtensionEvent(t, instance, "onebot11qq.status")
	writeOneBotPacket(t, connection, map[string]any{
		"post_type": "message", "message_type": "group", "sub_type": "normal",
		"time": 123, "self_id": 10001, "message_id": 77, "group_id": 23456, "user_id": 30003,
		"message": []map[string]any{
			{"type": "text", "data": map[string]any{"text": "hello"}},
			{"type": "image", "data": map[string]any{"file": "image.jpg"}},
		},
	})
	event := nextExtensionEvent(t, instance, "onebot11qq.message")
	if event.ID != "77" || event.Scope != "group:23456" || !event.Time.Equal(time.Unix(123, 0)) {
		t.Fatalf("lost event identity/scope/time: %+v", event)
	}
	var domain onebot11qq.Event
	if err := json.Unmarshal(event.Data, &domain); err != nil || domain.Message == nil ||
		len(domain.Message.Segments) != 2 || domain.Message.Segments[1].Type != onebot11qq.QQSegmentImage {
		t.Fatalf("lost QQ domain data: %+v / %v", domain, err)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"post_type": "message", "message_type": "private", "sub_type": "friend",
		"time": 124, "self_id": 10001, "message_id": 78, "user_id": 30003,
		"message": []map[string]any{{"type": "text", "data": map[string]any{"text": "private"}}},
	})
	if event := nextExtensionEvent(t, instance, "onebot11qq.message"); event.Scope != "private:30003" {
		t.Fatalf("incorrect private scope: %+v", event)
	}
	writeOneBotPacket(t, connection, map[string]any{
		"post_type": "request", "request_type": "group", "sub_type": "add", "time": 125,
		"self_id": 10001, "user_id": 30003, "group_id": 23456, "flag": "request-flag", "comment": "join",
	})
	if event := nextExtensionEvent(t, instance, "onebot11qq.request"); event.Scope != "group:23456" {
		t.Fatalf("incorrect request scope: %+v", event)
	}
	_ = connection.Close()
	nextExtensionEvent(t, instance, "onebot11qq.status")
	if status := instance.Status(); status.State != extension.StateRunning || *status.Connected {
		t.Fatalf("disconnect incorrectly ended extension: %+v", status)
	}
	connectReverseWS(t, address, "10001", "token")
	nextExtensionEvent(t, instance, "onebot11qq.status")
	if !*instance.Status().Connected {
		t.Fatal("reconnect did not restore extension connection")
	}
}

func TestQQExtensionInvokeAndUnknownSendResult(t *testing.T) {
	instance, address := useQQExtension(t, onebot11qq.ReverseWSConfig{RequestTimeout: 100 * time.Millisecond})
	for _, arguments := range []string{"null", "[]", "{", "{}", `{"segments":[]} {}`} {
		if _, err := instance.Invoke(context.Background(), onebot11qq.CapabilitySendMessage, json.RawMessage(arguments)); !errors.Is(err, extension.ErrInvalidArguments) {
			t.Fatalf("accepted invalid invoke arguments %q: %v", arguments, err)
		}
	}
	if _, err := instance.Invoke(context.Background(), "unknown", json.RawMessage(`{}`)); !errors.Is(err, extension.ErrUnsupportedCapability) {
		t.Fatalf("unexpected unknown capability error: %v", err)
	}
	connection := connectReverseWS(t, address, "10001", "")
	nextExtensionEvent(t, instance, "onebot11qq.status")
	arguments, _ := json.Marshal(qqTestGroupMessage("test"))
	type invocation struct {
		result json.RawMessage
		err    error
	}
	invoke := func(ctx context.Context) <-chan invocation {
		out := make(chan invocation, 1)
		go func() {
			result, err := instance.Invoke(ctx, onebot11qq.CapabilitySendMessage, arguments)
			out <- invocation{result, err}
		}()
		return out
	}
	result := invoke(context.Background())
	action := readOneBotAction(t, connection)
	writeOneBotPacket(t, connection, onebot11qq.OneBot11ActionResponse{
		Status: "ok", Echo: action.Echo, Data: json.RawMessage(`{"message_id":303}`),
	})
	outcome := <-result
	var sent onebot11qq.SendResult
	if err := json.Unmarshal(outcome.result, &sent); outcome.err != nil || err != nil || sent.MessageID != "303" {
		t.Fatalf("invoke lost send result: %+v / %v", outcome, err)
	}
	result = invoke(context.Background())
	readOneBotAction(t, connection)
	outcome = <-result
	var sendError *onebot11qq.SendError
	if !errors.As(outcome.err, &sendError) || !sendError.MayHaveSent || !errors.Is(outcome.err, context.DeadlineExceeded) {
		t.Fatalf("invoke lost unknown-send error: %v", outcome.err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := instance.Invoke(ctx, onebot11qq.CapabilitySendMessage, arguments); !errors.Is(err, context.Canceled) {
		t.Fatalf("invoke ignored cancellation: %v", err)
	}
}

func TestQQExtensionContextClosesBlockedEventDelivery(t *testing.T) {
	instance, err := onebot11qq.NewReverseWSExtension(onebot11qq.ReverseWSConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	if err := instance.Start(ctx); err != nil {
		t.Fatal(err)
	}
	address := instance.Address()
	connectReverseWS(t, "ws://"+address+"/onebot/v11/ws", "10001", "")
	// 故意不消费连接状态事件，取消时不能卡在转发通道上。
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for instance.Status().State != extension.StateClosed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if instance.Status().State != extension.StateClosed {
		t.Fatal("cancellation did not close extension")
	}
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, open := <-instance.Events():
		if open {
			t.Fatal("event delivery did not close after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("event delivery blocked cancellation")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("cancellation did not release listener: %v", err)
	}
	_ = listener.Close()
}
