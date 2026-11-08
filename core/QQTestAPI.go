package Core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	qq "github.com/caliniya/caf/NetClient/QQ"
)

const qqTestLogCapacity = 200

// QQTestConfig 仅用于临时测试，令牌不写入磁盘也不返回给前端。
type QQTestConfig struct {
	Address        string `json:"address"`
	Path           string `json:"path"`
	AccessToken    string `json:"accessToken"`
	SelfID         string `json:"selfId"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type QQTestState struct {
	Running        bool             `json:"running"`
	Address        string           `json:"address"`
	Path           string           `json:"path"`
	ExpectedSelfID string           `json:"expectedSelfId"`
	AuthEnabled    bool             `json:"authEnabled"`
	TimeoutSeconds int              `json:"timeoutSeconds"`
	URLs           []string         `json:"urls"`
	Info           qq.ClientInfo    `json:"info"`
	Entries        []QQTestLogEntry `json:"entries"`
	LastID         uint64           `json:"lastId"`
}

type QQTestLogEntry struct {
	ID          uint64 `json:"id"`
	Time        string `json:"time"`
	Kind        string `json:"kind"`
	Summary     string `json:"summary"`
	Detail      string `json:"detail,omitempty"`
	MessageType string `json:"messageType,omitempty"`
	TargetID    string `json:"targetId,omitempty"`
}

type QQTestSendResult struct {
	MessageID string `json:"messageId"`
}

type qqTestSession struct {
	client   *qq.ReverseWSClient
	cancel   context.CancelFunc
	finished chan struct{}
}

// GetQQTestState 返回状态与游标之后的新日志，切换页面不会停止适配器。
func (c *Core) GetQQTestState(afterID uint64) QQTestState {
	c.qqTestMu.Lock()
	defer c.qqTestMu.Unlock()
	return c.qqTestSnapshotLocked(afterID)
}

func (c *Core) qqTestSnapshotLocked(afterID uint64) QQTestState {
	state := c.qqTestState
	if c.qqTest != nil {
		state.Info = c.qqTest.client.Info()
	}
	state.URLs = append([]string{}, state.URLs...)
	state.Entries = []QQTestLogEntry{}
	state.LastID = c.qqTestLogID
	for _, entry := range c.qqTestLog {
		if entry.ID > afterID {
			state.Entries = append(state.Entries, entry)
		}
	}
	return state
}

func (c *Core) StartQQTestAdapter(config QQTestConfig) (QQTestState, error) {
	c.qqTestLifecycle.Lock()
	defer c.qqTestLifecycle.Unlock()
	c.qqTestMu.Lock()
	running := c.qqTest != nil
	c.qqTestMu.Unlock()
	if running {
		return QQTestState{}, errors.New("测试适配器已启动，请先停止再修改配置")
	}
	config.Address, config.Path = strings.TrimSpace(config.Address), strings.TrimSpace(config.Path)
	config.SelfID = strings.TrimSpace(config.SelfID)
	if config.Address == "" {
		config.Address = "0.0.0.0:6701"
	}
	if config.Path == "" {
		config.Path = "/onebot/v11/ws"
	}
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 30
	}
	if config.TimeoutSeconds < 1 || config.TimeoutSeconds > 300 {
		return QQTestState{}, errors.New("发送超时必须为 1 至 300 秒")
	}
	client, err := qq.NewReverseWSClient(qq.ReverseWSConfig{
		Address: config.Address, Path: config.Path, AccessToken: config.AccessToken,
		SelfID: config.SelfID, RequestTimeout: time.Duration(config.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return QQTestState{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := client.Start(ctx); err != nil {
		cancel()
		_ = client.Close()
		return QQTestState{}, err
	}
	session := &qqTestSession{client: client, cancel: cancel, finished: make(chan struct{})}
	c.qqTestMu.Lock()
	c.qqTest = session
	c.qqTestState = QQTestState{
		Running: true, Address: client.Address(), Path: config.Path,
		ExpectedSelfID: config.SelfID, AuthEnabled: config.AccessToken != "",
		TimeoutSeconds: config.TimeoutSeconds, Info: client.Info(),
		URLs: qqTestConnectionURLs(client.Address(), config.Path),
	}
	c.qqTestAppendLogLocked(QQTestLogEntry{
		Kind: "started", Summary: "适配器已启动，等待 OneBot 11 反向连接",
		Detail: "监听地址：" + client.Address() + "\n路径：" + config.Path,
	})
	c.qqTestMu.Unlock()
	go c.consumeQQTestEvents(session)
	return c.GetQQTestState(0), nil
}

// StopQQTestAdapter 同时结束事件消费者和待发送请求，可重复调用。
func (c *Core) StopQQTestAdapter() (QQTestState, error) {
	c.qqTestLifecycle.Lock()
	defer c.qqTestLifecycle.Unlock()
	c.qqTestMu.Lock()
	session := c.qqTest
	if session == nil {
		state := c.qqTestSnapshotLocked(0)
		c.qqTestMu.Unlock()
		return state, nil
	}
	c.qqTest = nil
	c.qqTestState.Running = false
	c.qqTestState.Info = session.client.Info()
	c.qqTestState.Info.Connected, c.qqTestState.Info.Online = false, nil
	c.qqTestMu.Unlock()
	session.cancel()
	err := session.client.Close()
	<-session.finished
	c.qqTestMu.Lock()
	c.qqTestAppendLogLocked(QQTestLogEntry{Kind: "stopped", Summary: "适配器已停止"})
	state := c.qqTestSnapshotLocked(0)
	c.qqTestMu.Unlock()
	return state, err
}

func (c *Core) ClearQQTestLog() QQTestState {
	c.qqTestMu.Lock()
	defer c.qqTestMu.Unlock()
	c.qqTestLog = nil
	return c.qqTestSnapshotLocked(0)
}

// SendQQTestMessage 只发送用户在测试页填写的正文，不调用模型。
func (c *Core) SendQQTestMessage(messageType, targetID, text string) (QQTestSendResult, error) {
	if strings.TrimSpace(text) == "" {
		return QQTestSendResult{}, errors.New("消息正文不能为空")
	}
	targetID = strings.TrimSpace(targetID)
	message := apiImpl.QQMessage{
		Type: apiImpl.QQMessageType(messageType), SentBySelf: true,
		Segments: []apiImpl.QQMessageSegment{apiImpl.QQText(text)},
	}
	switch message.Type {
	case apiImpl.QQMessagePrivate:
		message.UserID = targetID
	case apiImpl.QQMessageGroup:
		message.GroupID = targetID
	default:
		return QQTestSendResult{}, errors.New("请选择私聊或群聊")
	}
	if _, err := message.ToOneBot11Action(nil); err != nil {
		return QQTestSendResult{}, err
	}
	c.qqTestMu.Lock()
	session := c.qqTest
	c.qqTestMu.Unlock()
	if session == nil {
		return QQTestSendResult{}, errors.New("请先启动测试适配器")
	}
	message.SelfID = session.client.Info().SelfID
	result, err := session.client.SendMessage(context.Background(), message)
	entry := QQTestLogEntry{
		Kind: "send", MessageType: messageType, TargetID: targetID,
		Summary: "发送至" + qqTestTargetName(message.Type) + " " + targetID + "：" + text,
	}
	if err != nil {
		entry.Kind, entry.Summary = "send_error", err.Error()
	}
	entry.Detail = qqTestJSON(struct {
		Message apiImpl.QQMessage `json:"message"`
		Result  qq.SendResult     `json:"result"`
		Error   string            `json:"error,omitempty"`
	}{Message: message, Result: result, Error: errorText(err)})
	c.qqTestMu.Lock()
	c.qqTestAppendLogLocked(entry)
	c.qqTestMu.Unlock()
	if err != nil {
		return QQTestSendResult{}, err
	}
	return QQTestSendResult{MessageID: result.MessageID}, nil
}

func (c *Core) consumeQQTestEvents(session *qqTestSession) {
	defer close(session.finished)
	for event := range session.client.Events() {
		entry := qqTestEventLog(event)
		c.qqTestMu.Lock()
		if c.qqTest == session {
			c.qqTestState.Info = session.client.Info()
			c.qqTestAppendLogLocked(entry)
		}
		c.qqTestMu.Unlock()
	}
	c.qqTestMu.Lock()
	if c.qqTest == session {
		c.qqTest = nil
		c.qqTestState.Running = false
		c.qqTestState.Info = session.client.Info()
		c.qqTestAppendLogLocked(QQTestLogEntry{Kind: "stopped", Summary: "适配器已结束"})
	}
	c.qqTestMu.Unlock()
	session.cancel()
}

func (c *Core) qqTestAppendLogLocked(entry QQTestLogEntry) {
	c.qqTestLogID++
	entry.ID = c.qqTestLogID
	entry.Time = time.Now().UTC().Format(time.RFC3339Nano)
	entry.Summary = qqTestLimitText(entry.Summary, 400)
	entry.Detail = qqTestLimitText(entry.Detail, 12000)
	if len(c.qqTestLog) == qqTestLogCapacity {
		copy(c.qqTestLog, c.qqTestLog[1:])
		c.qqTestLog[len(c.qqTestLog)-1] = entry
	} else {
		c.qqTestLog = append(c.qqTestLog, entry)
	}
}

func qqTestEventLog(event qq.Event) QQTestLogEntry {
	entry := QQTestLogEntry{Kind: string(event.Type), Detail: qqTestJSON(event)}
	switch event.Type {
	case qq.EventMessage:
		if event.Message == nil {
			break
		}
		message := event.Message
		entry.MessageType = string(message.Type)
		entry.TargetID = message.UserID
		if message.Type == apiImpl.QQMessageGroup {
			entry.TargetID = message.GroupID
		}
		name := message.UserID
		if message.Sender != nil {
			if message.Sender.Card != "" {
				name = message.Sender.Card + "（" + message.Sender.UserID + "）"
			} else if message.Sender.Nickname != "" {
				name = message.Sender.Nickname + "（" + message.Sender.UserID + "）"
			}
		}
		if message.SentBySelf {
			name = "自身消息"
		}
		parts := make([]string, 0, len(message.Segments))
		for _, segment := range message.Segments {
			if segment.Type == apiImpl.QQSegmentText {
				text, _ := segment.Data["text"].(string)
				parts = append(parts, text)
			} else {
				parts = append(parts, "["+string(segment.Type)+"]")
			}
		}
		entry.Summary = qqTestTargetName(message.Type) + " " + entry.TargetID + " · " + name + "：" + strings.Join(parts, "")
	case qq.EventRequest:
		if event.Request != nil {
			kind := "好友申请"
			if event.Request.Type == "group" {
				kind = "群申请"
			}
			entry.Summary = kind + " · " + event.Request.UserID + "：" + event.Request.Comment
		}
	case qq.EventNotice:
		if event.Notice != nil {
			entry.Summary = event.Notice.Type
			if event.Notice.GroupID != "" {
				entry.Summary += " · 群 " + event.Notice.GroupID
			}
			if event.Notice.UserID != "" {
				entry.Summary += " · 用户 " + event.Notice.UserID
			}
		}
	case qq.EventStatus:
		if event.Info != nil {
			entry.Summary = "反向连接已断开"
			if event.Info.Connected {
				entry.Summary = "反向连接已建立 · QQ " + event.Info.SelfID
				if event.Info.Online != nil {
					if *event.Info.Online {
						entry.Summary += " · 账号在线"
					} else {
						entry.Summary += " · 账号离线"
					}
				}
			} else if event.Info.LastError != "" {
				entry.Summary += "：" + event.Info.LastError
			}
		}
	case qq.EventError:
		entry.Summary = event.Error
	}
	if entry.Summary == "" {
		entry.Summary = string(event.Type)
	}
	return entry
}

func qqTestTargetName(messageType apiImpl.QQMessageType) string {
	if messageType == apiImpl.QQMessageGroup {
		return "群"
	}
	return "私聊"
}

func qqTestJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func qqTestLimitText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "\n…（内容已截断）"
	}
	return text
}

func qqTestConnectionURLs(address, path string) []string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return []string{}
	}
	hosts := []string{host}
	if host == "" || host == "0.0.0.0" || host == "::" {
		hosts = []string{"127.0.0.1"}
		if addresses, err := net.InterfaceAddrs(); err == nil {
			extra := map[string]bool{}
			for _, address := range addresses {
				if network, ok := address.(*net.IPNet); ok && !network.IP.IsLoopback() && network.IP.To4() != nil {
					extra[network.IP.String()] = true
				}
			}
			otherHosts := make([]string, 0, len(extra))
			for host := range extra {
				otherHosts = append(otherHosts, host)
			}
			sort.Strings(otherHosts)
			hosts = append(hosts, otherHosts...)
		}
	}
	urls := make([]string, 0, len(hosts))
	for _, host := range hosts {
		endpoint := url.URL{Scheme: "ws", Host: net.JoinHostPort(host, port), Path: path}
		urls = append(urls, endpoint.String())
	}
	return urls
}
