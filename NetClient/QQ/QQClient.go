package QQ

import (
	"context"
	"errors"
	"fmt"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// QQClient 对应一个机器人账号。客户端只负责收发，上层负责事件分发和聊天调度。
// 监听地址、反向连接或扫码登录等建立连接的方式由具体适配器提供。
type QQClient interface {
	Info() ClientInfo
	// Events 是单消费者事件流；上层应持续读取，再分发给界面和 Agent。
	// Close 后通道关闭；暂时掉线不会关闭，重新连接后继续输出事件。
	Events() <-chan Event
	SendMessage(context.Context, apiImpl.QQMessage) (SendResult, error)
	// Close 释放连接和资源，可重复调用。
	Close() error
}

type ClientInfo struct {
	SelfID    string `json:"self_id"`
	Nickname  string `json:"nickname,omitempty"`
	Adapter   string `json:"adapter"`
	Connected bool   `json:"connected"`
	// Online 是客户端上报的 QQ 状态；nil 表示尚未得知或连接已断开。
	Online    *bool  `json:"online,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type SendResult struct {
	MessageID string `json:"message_id"`
}

var (
	ErrClientClosed   = errors.New("QQ 客户端已关闭")
	ErrNotConnected   = errors.New("QQ 客户端尚未连接")
	ErrQQOffline      = errors.New("QQ 账号已离线")
	ErrEventQueueFull = errors.New("QQ 事件队列已满")
)

// SendError 区分明确未发送和发送结果未知。MayHaveSent 为 true 时不能自动重发，
// 因为对端可能已经发送消息，只是响应丢失或超时。
type SendError struct {
	Err         error
	MayHaveSent bool
}

func (err *SendError) Error() string {
	if err.MayHaveSent {
		return "QQ 消息发送结果未知：" + err.Err.Error()
	}
	return "QQ 消息发送失败：" + err.Err.Error()
}

func (err *SendError) Unwrap() error { return err.Err }

type ActionError struct {
	Action  string
	Status  string
	RetCode int
	Message string
}

func (err *ActionError) Error() string {
	return fmt.Sprintf("OneBot 11 动作 %s 返回 %s（retcode=%d）：%s", err.Action, err.Status, err.RetCode, err.Message)
}
