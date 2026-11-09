package QQ

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type reverseWSReply struct {
	response OneBot11ActionResponse
	err      error
}

// 每条连接独立保存请求关联表，旧连接的回包不能完成新连接上的发送。
type reverseWSSession struct {
	connection *websocket.Conn
	done       chan struct{}
	writer     chan struct{}
	mu         sync.Mutex
	closed     bool
	reason     error
	pending    map[string]chan reverseWSReply
}

func newReverseWSSession(connection *websocket.Conn) *reverseWSSession {
	session := &reverseWSSession{
		connection: connection, done: make(chan struct{}),
		writer: make(chan struct{}, 1), pending: make(map[string]chan reverseWSReply),
	}
	session.writer <- struct{}{}
	return session
}

func (session *reverseWSSession) close(reason error) {
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return
	}
	if reason == nil {
		reason = ErrNotConnected
	}
	session.closed, session.reason = true, reason
	pending := session.pending
	session.pending = nil
	close(session.done)
	session.mu.Unlock()
	_ = session.connection.Close()
	for _, reply := range pending {
		reply <- reverseWSReply{err: reason}
	}
}

func (session *reverseWSSession) closeReason() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.reason
}

func (session *reverseWSSession) register(echo string) (chan reverseWSReply, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return nil, session.reason
	}
	reply := make(chan reverseWSReply, 1)
	session.pending[echo] = reply
	return reply, nil
}

func (session *reverseWSSession) forget(echo string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	delete(session.pending, echo)
}

func (session *reverseWSSession) respond(echo string, response OneBot11ActionResponse) {
	session.mu.Lock()
	reply := session.pending[echo]
	delete(session.pending, echo)
	session.mu.Unlock()
	if reply != nil {
		reply <- reverseWSReply{response: response}
	}
}

// attempted 表示已经尝试写入网络；失败时无法保证对端没有收到请求。
func (session *reverseWSSession) write(ctx context.Context, payload []byte, timeout time.Duration) (attempted bool, err error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-session.done:
		return false, session.closeReason()
	case <-session.writer:
	}
	defer func() { session.writer <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if reason := session.closeReason(); reason != nil {
		return false, reason
	}
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := session.connection.SetWriteDeadline(deadline); err != nil {
		return false, err
	}
	// 取消正阻塞的写入会关闭当前连接；等待响应时取消仅移除该请求。
	stop := context.AfterFunc(ctx, func() { session.close(ctx.Err()) })
	err = session.connection.WriteMessage(websocket.TextMessage, payload)
	stop()
	if err == nil {
		err = ctx.Err()
	}
	return true, err
}

func (client *ReverseWSClient) SendMessage(ctx context.Context, message QQMessage) (SendResult, error) {
	fail := func(err error, attempted bool) (SendResult, error) {
		return SendResult{}, &SendError{Err: err, MayHaveSent: attempted}
	}
	if ctx == nil {
		return fail(fmt.Errorf("发送 QQ 消息必须提供 context"), false)
	}
	if err := ctx.Err(); err != nil {
		return fail(err, false)
	}
	ctx, cancel := context.WithTimeout(ctx, client.config.RequestTimeout)
	defer cancel()
	client.mu.RLock()
	session, info, closed := client.session, copyClientInfo(client.info), client.closed
	client.mu.RUnlock()
	if closed {
		return fail(ErrClientClosed, false)
	}
	if session == nil {
		return fail(ErrNotConnected, false)
	}
	if info.Online != nil && !*info.Online {
		return fail(ErrQQOffline, false)
	}
	if message.SelfID != "" && message.SelfID != info.SelfID {
		return fail(fmt.Errorf("消息账号 %s 与当前连接账号 %s 不一致", message.SelfID, info.SelfID), false)
	}
	echo := fmt.Sprintf("caf-send-%d", client.nextEcho.Add(1))
	echoJSON, _ := json.Marshal(echo)
	action, err := message.ToOneBot11Action(echoJSON)
	if err != nil {
		return fail(err, false)
	}
	payload, err := json.Marshal(action)
	if err != nil {
		return fail(err, false)
	}
	reply, err := session.register(echo)
	if err != nil {
		return fail(err, false)
	}
	defer session.forget(echo)
	attempted, err := session.write(ctx, payload, client.config.WriteTimeout)
	if err != nil {
		if attempted {
			client.disconnect(session, err)
		}
		return fail(err, attempted)
	}
	var result reverseWSReply
	select {
	case result = <-reply:
	case <-ctx.Done():
		// 已收到明确结果时优先返回结果，避免把成功响应误报为超时。
		select {
		case result = <-reply:
		default:
			return fail(ctx.Err(), true)
		}
	case <-session.done:
		select {
		case result = <-reply:
		default:
			return fail(session.closeReason(), true)
		}
	}
	if result.err != nil {
		return fail(result.err, true)
	}
	response := result.response
	if response.Status != "ok" || response.RetCode != 0 {
		description := response.Message
		if description == "" {
			description = response.Wording
		}
		if description == "" {
			description = "对端没有提供错误说明"
		}
		return fail(&ActionError{
			Action: action.Action, Status: response.Status, RetCode: response.RetCode, Message: description,
		}, response.Status != "failed")
	}
	var data struct {
		MessageID *OneBot11ID `json:"message_id"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return fail(fmt.Errorf("解析 OneBot 11 发送结果失败：%w", err), true)
	}
	if data.MessageID == nil {
		return fail(fmt.Errorf("OneBot 11 发送成功响应缺少 message_id"), true)
	}
	return SendResult{MessageID: data.MessageID.String()}, nil
}

func (client *ReverseWSClient) pingSession(session *reverseWSSession) {
	defer client.workers.Done()
	ticker := time.NewTicker(client.config.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.done:
			return
		case <-client.done:
			return
		case <-ticker.C:
			if err := session.connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(client.config.WriteTimeout)); err != nil {
				client.disconnect(session, err)
				return
			}
		}
	}
}
