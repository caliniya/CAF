package Onebot11QQ

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var _ MessageReader = (*ReverseWSClient)(nil)
var _ MessageReader = (*ReverseWSExtension)(nil)

// GetMessage 在已有反向 WebSocket 上调用标准 get_msg，不建立新连接。
func (client *ReverseWSClient) GetMessage(ctx context.Context, messageID string) (QQMessage, error) {
	if ctx == nil {
		return QQMessage{}, errors.New("读取 QQ 消息必须提供 context")
	}
	id, err := qqOneBotID("message_id", messageID, false)
	if err != nil || messageID == "" {
		return QQMessage{}, errors.New("引用消息 ID 必须是有效的 64 位整数")
	}
	ctx, cancel := context.WithTimeout(ctx, client.config.RequestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return QQMessage{}, err
	}
	client.mu.RLock()
	session, info, closed := client.session, copyClientInfo(client.info), client.closed
	client.mu.RUnlock()
	if closed {
		return QQMessage{}, ErrClientClosed
	}
	if session == nil {
		return QQMessage{}, ErrNotConnected
	}
	echo := fmt.Sprintf("caf-get-%d", client.nextEcho.Add(1))
	echoJSON, _ := json.Marshal(echo)
	payload, err := json.Marshal(OneBot11ActionRequest{Action: "get_msg", Params: map[string]any{"message_id": id}, Echo: echoJSON})
	if err != nil {
		return QQMessage{}, err
	}
	reply, err := session.register(echo)
	if err != nil {
		return QQMessage{}, err
	}
	defer session.forget(echo)
	if attempted, err := session.write(ctx, payload, client.config.WriteTimeout); err != nil {
		if attempted {
			client.disconnect(session, err)
		}
		return QQMessage{}, err
	}
	var result reverseWSReply
	select {
	case result = <-reply:
	case <-ctx.Done():
		return QQMessage{}, ctx.Err()
	case <-session.done:
		return QQMessage{}, session.closeReason()
	}
	if result.err != nil {
		return QQMessage{}, result.err
	}
	response := result.response
	if response.Status != "ok" || response.RetCode != 0 {
		description := response.Message
		if description == "" {
			description = response.Wording
		}
		return QQMessage{}, &ActionError{Action: "get_msg", Status: response.Status, RetCode: response.RetCode, Message: description}
	}
	var data struct {
		Time        int64           `json:"time"`
		MessageID   *OneBot11ID     `json:"message_id"`
		MessageType string          `json:"message_type"`
		UserID      OneBot11ID      `json:"user_id"`
		GroupID     OneBot11ID      `json:"group_id"`
		Sender      *OneBot11Sender `json:"sender"`
		Message     OneBot11Message `json:"message"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return QQMessage{}, fmt.Errorf("解析引用消息失败：%w", err)
	}
	if data.MessageID == nil || *data.MessageID != id || (data.MessageType != OneBot11MessagePrivate && data.MessageType != OneBot11MessageGroup) {
		return QQMessage{}, errors.New("引用消息的 ID 或类型与请求不一致")
	}
	if data.UserID == 0 && data.Sender != nil {
		data.UserID = data.Sender.UserID
	}
	if data.UserID <= 0 {
		return QQMessage{}, errors.New("引用消息缺少发送者 QQ 号")
	}
	message := QQMessage{ID: id.String(), SelfID: info.SelfID, Type: QQMessageType(data.MessageType),
		UserID: data.UserID.String(), GroupID: optionalOneBotID(data.GroupID), Time: data.Time, SentBySelf: data.UserID.String() == info.SelfID}
	if data.Sender != nil {
		message.Sender = &QQSender{UserID: data.UserID.String(), Nickname: data.Sender.Nickname, Card: data.Sender.Card}
	}
	for _, segment := range data.Message {
		message.Segments = append(message.Segments, QQMessageSegment{Type: QQSegmentType(segment.Type), Data: segment.Data})
	}
	if err := message.validate(); err != nil {
		return QQMessage{}, err
	}
	return message, nil
}
