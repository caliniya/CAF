package Onebot11QQ

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"time"
)

var errWSAccountMismatch = errors.New("OneBot 11 事件账号与连接账号不一致")

func (client *ReverseWSClient) readEvents(session *reverseWSSession) {
	defer client.workers.Done()
	for {
		frameType, payload, err := session.connection.ReadMessage()
		if err != nil {
			if reason := session.closeReason(); reason != nil {
				err = reason
			}
			client.disconnect(session, err)
			return
		}
		if err := session.connection.SetReadDeadline(time.Now().Add(client.config.PongTimeout)); err != nil {
			client.disconnect(session, err)
			return
		}
		client.mu.RLock()
		current := !client.closed && client.session == session
		client.mu.RUnlock()
		if !current {
			return
		}
		if frameType != websocket.TextMessage && frameType != websocket.BinaryMessage {
			continue
		}
		if err := client.handlePayload(session, payload); err != nil {
			if errors.Is(err, errWSAccountMismatch) || errors.Is(err, ErrEventQueueFull) {
				client.disconnect(session, err)
				return
			}
			client.reportError(err)
		}
	}
}

func (client *ReverseWSClient) handlePayload(session *reverseWSSession, payload []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return fmt.Errorf("解析 OneBot 11 WebSocket 数据失败：%w", err)
	}
	if fields == nil {
		return errors.New("OneBot 11 WebSocket 数据必须是 JSON 对象")
	}
	if _, exists := fields["post_type"]; !exists {
		// 动作响应和事件共用连接，响应直接完成等待请求，不进入事件队列。
		var response OneBot11ActionResponse
		if err := json.Unmarshal(payload, &response); err != nil {
			return fmt.Errorf("解析 OneBot 11 动作响应失败：%w", err)
		}
		var echo string
		if err := json.Unmarshal(response.Echo, &echo); err != nil || echo == "" {
			return errors.New("OneBot 11 动作响应缺少原请求的字符串 echo")
		}
		session.respond(echo, response)
		return nil
	}
	var header OneBot11EventHeader
	if err := json.Unmarshal(payload, &header); err != nil {
		return err
	}
	client.mu.RLock()
	current, selfID := client.session == session, client.info.SelfID
	client.mu.RUnlock()
	if !current {
		return nil
	}
	if header.SelfID <= 0 || header.SelfID.String() != selfID {
		return errWSAccountMismatch
	}
	event := Event{SelfID: selfID, Time: header.Time, Raw: bytes.Clone(payload)}
	switch header.PostType {
	case OneBot11PostMessage, OneBot11PostMessageSent:
		var incoming OneBot11MessageEvent
		if err := json.Unmarshal(payload, &incoming); err != nil {
			return fmt.Errorf("解析 QQ 消息失败：%w", err)
		}
		message, err := incoming.ToQQMessage()
		if err != nil {
			return err
		}
		// 部分实现把自身消息也上报为 message，通过发送者账号识别方向。
		if incoming.UserID == incoming.SelfID || (incoming.Sender != nil && incoming.Sender.UserID == incoming.SelfID) {
			message.SentBySelf = true
		}
		event.Type, event.Message = EventMessage, &message
		client.updateOwnNickname(session, incoming)
	case OneBot11PostRequest:
		var incoming OneBot11RequestEvent
		if err := json.Unmarshal(payload, &incoming); err != nil {
			return fmt.Errorf("解析 QQ 申请事件失败：%w", err)
		}
		if incoming.RequestType == "" || incoming.UserID <= 0 || incoming.Flag == "" {
			return errors.New("QQ 申请事件缺少 request_type、user_id 或 flag")
		}
		event.Type, event.Request = EventRequest, &Request{
			Type: incoming.RequestType, SubType: incoming.SubType,
			UserID: incoming.UserID.String(), GroupID: optionalOneBotID(incoming.GroupID),
			Comment: incoming.Comment, Flag: incoming.Flag,
		}
	case OneBot11PostNotice:
		var incoming OneBot11NoticeEvent
		if err := json.Unmarshal(payload, &incoming); err != nil {
			return fmt.Errorf("解析 QQ 通知失败：%w", err)
		}
		if incoming.NoticeType == "" {
			return errors.New("QQ 通知缺少 notice_type")
		}
		event.Type, event.Notice = EventNotice, &Notice{
			Type: incoming.NoticeType, SubType: incoming.SubType,
			UserID: optionalOneBotID(incoming.UserID), GroupID: optionalOneBotID(incoming.GroupID),
			OperatorID: optionalOneBotID(incoming.OperatorID), TargetID: optionalOneBotID(incoming.TargetID),
			MessageID: optionalOneBotID(incoming.MessageID),
		}
	case OneBot11PostMetaEvent:
		return client.handleMetaEvent(session, payload)
	default:
		return fmt.Errorf("不支持的 OneBot 11 post_type %q", header.PostType)
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.closed || client.session != session {
		return nil
	}
	if !client.queueEvent(event) {
		return ErrEventQueueFull
	}
	return nil
}

func optionalOneBotID(id OneBot11ID) string {
	if id == 0 {
		return ""
	}
	return id.String()
}

func (client *ReverseWSClient) handleMetaEvent(session *reverseWSSession, payload []byte) error {
	var incoming OneBot11MetaEvent
	if err := json.Unmarshal(payload, &incoming); err != nil {
		return err
	}
	var status struct {
		Online *bool `json:"online"`
	}
	if len(incoming.Status) > 0 {
		if err := json.Unmarshal(incoming.Status, &status); err != nil {
			return fmt.Errorf("解析 QQ 在线状态失败：%w", err)
		}
	}
	if status.Online == nil {
		return nil
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed || client.session != session {
		return nil
	}
	if client.info.Online != nil && *client.info.Online == *status.Online {
		return nil
	}
	client.info.Online = status.Online
	info := copyClientInfo(client.info)
	client.queueControl(Event{
		Type: EventStatus, SelfID: info.SelfID, Time: incoming.Time, Info: &info, Raw: bytes.Clone(payload),
	})
	return nil
}

func (client *ReverseWSClient) updateOwnNickname(session *reverseWSSession, event OneBot11MessageEvent) {
	if event.Sender == nil || event.Sender.UserID != event.SelfID || event.Sender.Nickname == "" {
		return
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed || client.session != session || client.info.Nickname == event.Sender.Nickname {
		return
	}
	client.info.Nickname = event.Sender.Nickname
	info := copyClientInfo(client.info)
	client.queueControl(Event{Type: EventStatus, SelfID: info.SelfID, Time: event.Time, Info: &info})
}
