package Onebot11QQ

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// ReverseWSConfig 配置单账号的 OneBot 11 Universal 反向 WebSocket 端点。
// SelfID 为空时在首次成功连接时绑定账号，后续只接受同一账号重新连接。
type ReverseWSConfig struct {
	Address          string
	Path             string
	AccessToken      string
	SelfID           string
	RequestTimeout   time.Duration
	WriteTimeout     time.Duration
	HandshakeTimeout time.Duration
	PingInterval     time.Duration
	PongTimeout      time.Duration
	MaxMessageBytes  int64
	EventBuffer      int
}

// ReverseWSClient 同时实现 QQClient 和 http.Handler。
// 可调用 Start 自行监听，也可挂载到已有 HTTP 服务；不依赖 NapCat 专有 API。
type ReverseWSClient struct {
	config ReverseWSConfig

	mu         sync.RWMutex
	info       ClientInfo
	session    *reverseWSSession
	server     *http.Server
	listener   net.Listener
	closed     bool
	closeOnce  sync.Once
	closeError error
	done       chan struct{}
	workers    sync.WaitGroup
	nextEcho   atomic.Uint64

	events      chan Event
	eventMu     sync.Mutex
	eventQueue  []Event
	lastControl *Event
	eventReady  chan struct{}
}

var _ QQClient = (*ReverseWSClient)(nil)
var _ http.Handler = (*ReverseWSClient)(nil)

func NewReverseWSClient(config ReverseWSConfig) (*ReverseWSClient, error) {
	if config.Address == "" {
		config.Address = "127.0.0.1:6701"
	}
	if config.Path == "" {
		config.Path = "/onebot/v11/ws"
	}
	path, err := url.ParseRequestURI(config.Path)
	if err != nil || !strings.HasPrefix(config.Path, "/") || strings.HasPrefix(config.Path, "//") ||
		path == nil || path.RawQuery != "" || path.Fragment != "" || path.Path != config.Path {
		return nil, errors.New("反向 WS 路径必须是以 / 开头的 URL 路径")
	}
	if config.SelfID != "" {
		config.SelfID, err = canonicalSelfID(config.SelfID)
		if err != nil {
			return nil, err
		}
	}
	if config.RequestTimeout < 0 || config.WriteTimeout < 0 || config.HandshakeTimeout < 0 ||
		config.PingInterval < 0 || config.PongTimeout < 0 ||
		config.MaxMessageBytes < 0 || config.EventBuffer < 0 {
		return nil, errors.New("反向 WS 超时、消息大小和事件队列容量不能为负数")
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 10 * time.Second
	}
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = 10 * time.Second
	}
	if config.PingInterval == 0 {
		config.PingInterval = 30 * time.Second
	}
	if config.PongTimeout == 0 {
		config.PongTimeout = 90 * time.Second
	}
	if config.PongTimeout <= config.PingInterval {
		return nil, errors.New("反向 WS 的 PongTimeout 必须大于 PingInterval")
	}
	if config.MaxMessageBytes == 0 {
		config.MaxMessageBytes = 4 << 20
	}
	if config.EventBuffer == 0 {
		config.EventBuffer = 256
	}
	client := &ReverseWSClient{
		config: config,
		info:   ClientInfo{SelfID: config.SelfID, Adapter: "onebot11-reverse-ws"},
		done:   make(chan struct{}), events: make(chan Event),
		eventReady: make(chan struct{}, 1),
	}
	client.workers.Add(1)
	go client.deliverEvents()
	return client, nil
}

func (client *ReverseWSClient) Info() ClientInfo {
	client.mu.RLock()
	defer client.mu.RUnlock()
	return copyClientInfo(client.info)
}

func copyClientInfo(info ClientInfo) ClientInfo {
	if info.Online != nil {
		online := *info.Online
		info.Online = &online
	}
	return info
}

func (client *ReverseWSClient) Events() <-chan Event { return client.events }

// Start 开始监听后立即返回。ctx 取消时关闭整个客户端。
func (client *ReverseWSClient) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("启动反向 WS 必须提供 context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return ErrClientClosed
	}
	if client.server != nil {
		client.mu.Unlock()
		return errors.New("反向 WS 服务已经启动")
	}
	listener, err := net.Listen("tcp", client.config.Address)
	if err != nil {
		client.mu.Unlock()
		return fmt.Errorf("监听反向 WS 失败：%w", err)
	}
	server := &http.Server{
		Handler: client, ReadHeaderTimeout: client.config.HandshakeTimeout,
	}
	client.listener, client.server = listener, server
	client.mu.Unlock()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			client.reportError(err)
			_ = client.Close()
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-client.done:
		}
	}()
	return nil
}

// Address 返回实际监听地址，支持配置端口为 0 时由系统分配端口。
func (client *ReverseWSClient) Address() string {
	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.listener != nil {
		return client.listener.Addr().String()
	}
	return client.config.Address
}

func (client *ReverseWSClient) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != client.config.Path {
		http.NotFound(writer, request)
		return
	}
	if !client.authorized(request) {
		http.Error(writer, "OneBot 11 access token is invalid", http.StatusUnauthorized)
		return
	}
	role := strings.TrimSpace(request.Header.Get("X-Client-Role"))
	if role != "" && !strings.EqualFold(role, "Universal") {
		http.Error(writer, "Use a Universal reverse WebSocket connection", http.StatusBadRequest)
		return
	}
	selfID, err := canonicalSelfID(request.Header.Get("X-Self-ID"))
	if err != nil {
		http.Error(writer, "X-Self-ID must identify the bot account", http.StatusBadRequest)
		return
	}
	client.mu.RLock()
	closed, boundID := client.closed, client.info.SelfID
	client.mu.RUnlock()
	if closed {
		http.Error(writer, "QQ client is closed", http.StatusServiceUnavailable)
		return
	}
	if boundID != "" && boundID != selfID {
		http.Error(writer, "This endpoint belongs to another bot account", http.StatusConflict)
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: client.config.HandshakeTimeout}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	connection.SetReadLimit(client.config.MaxMessageBytes)
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(client.config.PongTimeout))
	})
	if err := connection.SetReadDeadline(time.Now().Add(client.config.PongTimeout)); err != nil {
		_ = connection.Close()
		return
	}
	session := newReverseWSSession(connection)
	client.mu.Lock()
	// 握手期间可能关闭服务，或另一连接先完成账号绑定。
	if client.closed || (client.info.SelfID != "" && client.info.SelfID != selfID) {
		client.mu.Unlock()
		session.close(ErrClientClosed)
		return
	}
	previous := client.session
	client.session = session
	client.info.SelfID, client.info.Connected = selfID, true
	client.info.Online, client.info.LastError = nil, ""
	info := copyClientInfo(client.info)
	client.workers.Add(2)
	client.queueControl(Event{Type: EventStatus, SelfID: selfID, Time: time.Now().Unix(), Info: &info})
	client.mu.Unlock()
	if previous != nil {
		previous.close(errors.New("OneBot 11 连接已被同一账号的新连接替换"))
	}
	go client.readEvents(session)
	go client.pingSession(session)
}

func canonicalSelfID(text string) (string, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || value <= 0 {
		return "", errors.New("机器人账号必须是正的 64 位整数")
	}
	return strconv.FormatInt(value, 10), nil
}

func (client *ReverseWSClient) authorized(request *http.Request) bool {
	if client.config.AccessToken == "" {
		return true
	}
	token := request.URL.Query().Get("access_token")
	if header := request.Header.Get("Authorization"); header != "" {
		scheme, credentials, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			return false
		}
		token = credentials
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(client.config.AccessToken)) == 1
}

func (client *ReverseWSClient) disconnect(session *reverseWSSession, reason error) {
	session.close(reason)
	client.mu.Lock()
	if client.session != session {
		client.mu.Unlock()
		return
	}
	client.session = nil
	client.info.Connected, client.info.Online = false, nil
	if reason != nil {
		client.info.LastError = reason.Error()
	}
	info := copyClientInfo(client.info)
	client.queueControl(Event{Type: EventStatus, SelfID: info.SelfID, Time: time.Now().Unix(), Info: &info})
	client.mu.Unlock()
}

func (client *ReverseWSClient) Close() error {
	client.closeOnce.Do(func() {
		client.mu.Lock()
		client.closed = true
		session, server := client.session, client.server
		client.session = nil
		client.info.Connected, client.info.Online = false, nil
		close(client.done)
		client.mu.Unlock()
		if session != nil {
			session.close(ErrClientClosed)
		}
		if server != nil {
			client.closeError = server.Close()
		}
		client.workers.Wait()
	})
	return client.closeError
}

// 消息和请求排队交付，网络读取不会被模型处理阻塞。
// 消费者长期不读取导致队列满时断开连接，并保留最后一个状态/错误供上层获知。
func (client *ReverseWSClient) queueEvent(event Event) bool {
	client.eventMu.Lock()
	defer client.eventMu.Unlock()
	select {
	case <-client.done:
		return false
	default:
	}
	if len(client.eventQueue) >= client.config.EventBuffer {
		return false
	}
	if client.lastControl != nil {
		client.eventQueue = append(client.eventQueue, *client.lastControl)
		client.lastControl = nil
		if len(client.eventQueue) >= client.config.EventBuffer {
			return false
		}
	}
	client.eventQueue = append(client.eventQueue, event)
	select {
	case client.eventReady <- struct{}{}:
	default:
	}
	return true
}

func (client *ReverseWSClient) queueControl(event Event) {
	client.eventMu.Lock()
	defer client.eventMu.Unlock()
	select {
	case <-client.done:
		return
	default:
	}
	if client.lastControl != nil {
		// 队列曾经溢出时只保留最新状态，避免重连后再交付旧的掉线状态。
		client.lastControl = &event
	} else if len(client.eventQueue) < client.config.EventBuffer {
		client.eventQueue = append(client.eventQueue, event)
	} else {
		client.lastControl = &event
	}
	select {
	case client.eventReady <- struct{}{}:
	default:
	}
}

func (client *ReverseWSClient) deliverEvents() {
	defer client.workers.Done()
	defer close(client.events)
	for {
		client.eventMu.Lock()
		var next *Event
		if len(client.eventQueue) > 0 {
			event := client.eventQueue[0]
			client.eventQueue[0] = Event{}
			client.eventQueue = client.eventQueue[1:]
			next = &event
		} else if client.lastControl != nil {
			next, client.lastControl = client.lastControl, nil
		}
		client.eventMu.Unlock()
		if next == nil {
			select {
			case <-client.eventReady:
				continue
			case <-client.done:
				return
			}
		}
		select {
		case client.events <- *next:
		case <-client.done:
			return
		}
	}
}

func (client *ReverseWSClient) reportError(err error) {
	info := client.Info()
	client.queueControl(Event{
		Type: EventError, SelfID: info.SelfID, Time: time.Now().Unix(), Error: err.Error(),
	})
}
