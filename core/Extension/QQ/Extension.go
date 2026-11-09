package QQ

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	extension "github.com/caliniya/caf/core/Extension"
)

const (
	ExtensionType         = "onebot11qq"
	CapabilitySendMessage = "send_message"
)

// Extension 是 QQ 扩展的契约。通用宿主只使用 extension.Extension；
// QQ 专用功能可以使用 Info / SendMessage，不依赖 OneBot 或 LagrangeGo。
type Extension interface {
	extension.Extension
	Info() ClientInfo
	SendMessage(context.Context, QQMessage) (SendResult, error)
}

// ReverseWSExtension 是 QQ 扩展的 OneBot 11 反向 WS 实现。
// 底层客户端的事件仅由该实现消费，宿主使用统一的扩展事件流。
type ReverseWSExtension struct {
	client *ReverseWSClient
	mu     sync.RWMutex
	state  extension.State
	err    string

	events     chan extension.Event
	done       chan struct{}
	workers    sync.WaitGroup
	closeOnce  sync.Once
	closeError error
}

var _ extension.Extension = (*ReverseWSExtension)(nil)
var _ Extension = (*ReverseWSExtension)(nil)

func NewReverseWSExtension(config ReverseWSConfig) (*ReverseWSExtension, error) {
	client, err := NewReverseWSClient(config)
	if err != nil {
		return nil, err
	}
	instance := &ReverseWSExtension{
		client: client, state: extension.StateStopped,
		events: make(chan extension.Event), done: make(chan struct{}),
	}
	instance.workers.Add(1)
	go instance.forwardEvents()
	return instance, nil
}

func OneBot11Descriptor() extension.Descriptor {
	return extension.Descriptor{
		Type: ExtensionType, Implementation: "onebot11-reverse-ws", Name: "Onebot11QQ",
		Description: "通过 OneBot 11 反向 WebSocket 接收 QQ 事件并发送消息",
		Parameters: []extension.ParameterField{
			{Key: "name", Label: "名称", Kind: "text", Required: true, Pattern: `^[\p{L}\p{N}_-]{1,32}$`, Help: "指令使用 /名称，同一客户端中名称不能重复。", EditableWhileRunning: true},
			{Key: "token", Label: "Token", Kind: "secret", Required: true, Placeholder: "连接令牌"},
			{Key: "selfId", Label: "QQ 号", Kind: "text", Pattern: `^[0-9]*$`, Placeholder: "留空时自动识别"},
			{Key: "port", Label: "监听端口", Kind: "integer", Required: true, Default: "6701", Minimum: 1, Maximum: 65535},
		},
		Information: []extension.InformationField{
			{Key: "status", Label: "适配器状态", InList: true},
			{Key: "address", Label: "连接地址"},
		},
		Capabilities: []extension.Capability{{
			Name: CapabilitySendMessage, Description: "向 QQ 私聊或群聊发送消息",
			Parameters: json.RawMessage(`{
				"type":"object",
				"required":["type","segments"],
				"properties":{
					"type":{"type":"string","enum":["private","group"]},
					"user_id":{"type":"string","description":"私聊对象的 QQ 号"},
					"group_id":{"type":"string","description":"群聊的群号"},
					"segments":{"type":"array","minItems":1,"items":{
						"type":"object","required":["type","data"],
						"properties":{"type":{"type":"string"},"data":{"type":"object"}}
					}}
				},
				"oneOf":[
					{"properties":{"type":{"const":"private"}},"required":["user_id"]},
					{"properties":{"type":{"const":"group"}},"required":["group_id"]}
				]
			}`),
		}},
	}
}

func (instance *ReverseWSExtension) Describe() extension.Descriptor { return OneBot11Descriptor() }

func (instance *ReverseWSExtension) UIValues() map[string]string {
	status := instance.Status()
	label := "未启动"
	if status.State == extension.StateRunning {
		label = "未连接"
		if status.Connected != nil && *status.Connected {
			label = "已连接"
		}
	}
	host, port, err := net.SplitHostPort(instance.Address())
	if err == nil && (host == "0.0.0.0" || host == "::") {
		host = "127.0.0.1"
	}
	address := instance.Address()
	if err == nil {
		address = net.JoinHostPort(host, port)
	}
	return map[string]string{"status": label, "address": "ws://" + address + instance.client.config.Path}
}

func (instance *ReverseWSExtension) Status() extension.Status {
	instance.mu.RLock()
	defer instance.mu.RUnlock()
	state, lastError := instance.state, instance.err
	info := instance.client.Info()
	if info.LastError != "" {
		lastError = info.LastError
	}
	connected := state == extension.StateRunning && info.Connected
	return extension.Status{State: state, Connected: &connected, LastError: lastError}
}

func (instance *ReverseWSExtension) Info() ClientInfo { return instance.client.Info() }

// Address 返回该实现的实际监听地址，支持端口为 0 时由系统分配。
func (instance *ReverseWSExtension) Address() string { return instance.client.Address() }

func (instance *ReverseWSExtension) Start(ctx context.Context) error {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	if instance.state == extension.StateClosed {
		return extension.ErrClosed
	}
	if instance.state == extension.StateRunning {
		return extension.ErrAlreadyStarted
	}
	if err := instance.client.Start(ctx); err != nil {
		return err
	}
	instance.state = extension.StateRunning
	go func() {
		select {
		case <-ctx.Done():
			_ = instance.Close()
		case <-instance.client.done:
			_ = instance.Close()
		case <-instance.done:
		}
	}()
	return nil
}

func (instance *ReverseWSExtension) Events() <-chan extension.Event { return instance.events }

func (instance *ReverseWSExtension) SendMessage(ctx context.Context, message QQMessage) (SendResult, error) {
	instance.mu.RLock()
	state := instance.state
	instance.mu.RUnlock()
	if state == extension.StateClosed {
		return SendResult{}, extension.ErrClosed
	}
	if state != extension.StateRunning {
		return SendResult{}, extension.ErrNotStarted
	}
	return instance.client.SendMessage(ctx, message)
}

func (instance *ReverseWSExtension) Invoke(ctx context.Context, capability string, arguments json.RawMessage) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w：必须提供调用 context", extension.ErrInvalidArguments)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if capability != CapabilitySendMessage {
		return nil, fmt.Errorf("%w：%s", extension.ErrUnsupportedCapability, capability)
	}
	var message QQMessage
	decoder := json.NewDecoder(strings.NewReader(string(arguments)))
	if err := decoder.Decode(&message); err != nil {
		return nil, fmt.Errorf("%w：%v", extension.ErrInvalidArguments, err)
	}
	if message.Segments == nil {
		return nil, fmt.Errorf("%w：必须提供 QQ 消息对象及 segments", extension.ErrInvalidArguments)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w：只能提供一个 JSON 对象", extension.ErrInvalidArguments)
	}
	// 路由和消息段校验由现有 QQ 发送实现完成，保留其错误类型。
	result, err := instance.SendMessage(ctx, message)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (instance *ReverseWSExtension) forwardEvents() {
	defer instance.workers.Done()
	defer close(instance.events)
	defer func() {
		instance.mu.Lock()
		instance.state = extension.StateClosed
		instance.mu.Unlock()
	}()
	for {
		select {
		case <-instance.done:
			return
		case <-instance.client.done:
			return
		case event, open := <-instance.client.Events():
			if !open {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				instance.mu.Lock()
				instance.err = fmt.Sprintf("编码 QQ 扩展事件失败：%v", err)
				instance.mu.Unlock()
				continue
			}
			if event.Type == EventError {
				instance.mu.Lock()
				instance.err = event.Error
				instance.mu.Unlock()
			}
			if event.Type == EventStatus && event.Info != nil && event.Info.Connected {
				instance.mu.Lock()
				instance.err = ""
				instance.mu.Unlock()
			}
			outgoing := extension.Event{
				Type: "qq." + string(event.Type),
				Time: time.Unix(event.Time, 0).UTC(), Data: data,
			}
			if event.Message != nil {
				outgoing.ID = event.Message.ID
				outgoing.Scope = qqMessageScope(*event.Message)
			} else if event.Request != nil {
				outgoing.Scope = qqEventScope(event.Request.GroupID, event.Request.UserID)
			} else if event.Notice != nil {
				outgoing.Scope = qqEventScope(event.Notice.GroupID, event.Notice.UserID)
			}
			select {
			case instance.events <- outgoing:
			case <-instance.done:
				return
			case <-instance.client.done:
				return
			}
		}
	}
}

func qqMessageScope(message QQMessage) string {
	if message.Type == QQMessageGroup {
		return qqEventScope(message.GroupID, "")
	}
	return qqEventScope("", message.UserID)
}

func qqEventScope(groupID, userID string) string {
	if groupID != "" {
		return "group:" + groupID
	}
	if userID != "" {
		return "private:" + userID
	}
	return ""
}

func (instance *ReverseWSExtension) Close() error {
	instance.closeOnce.Do(func() {
		instance.mu.Lock()
		instance.state = extension.StateClosed
		close(instance.done)
		instance.mu.Unlock()
		instance.closeError = instance.client.Close()
		instance.workers.Wait()
	})
	return instance.closeError
}
