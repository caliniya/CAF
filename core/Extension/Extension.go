package Extension

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Extension 是一个已配置的扩展实例，由宿主绑定到 Agent。
// Onebot11QQ、未来的 LagrangeGoQQ 等分别是可创建的扩展类型。
// 领域事件可以共用 qq.* 等命名空间。
// 实例 ID、配置持久化、Agent 绑定和权限策略由宿主管理。
type Extension interface {
	Describe() Descriptor
	Status() Status
	// UIValues 返回展示字段对应的纯文本快照，不包含创建参数中的秘密值。
	UIValues() map[string]string
	// Start 启动事件接收/能力服务，准备就绪后返回，不等待远端上线。
	// 每个实例启动一次；ctx 的生命周期属于宿主，取消时关闭扩展。
	Start(context.Context) error
	// Events 是单消费者事件流，宿主持续读取后分发给 Agent 或界面。
	// 临时断线不关闭通道；Close 完成后通道关闭。
	// 不产生事件的扩展也提供通道，并在关闭时结束。
	Events() <-chan Event
	// Invoke 调用 Describe 声明的能力；arguments/result 是 JSON 数据。
	// ctx 控制本次调用，权限校验和是否提供给模型由宿主决定。
	// 返回的错误应保留实现自身的错误类型，例如发送结果未知。
	Invoke(ctx context.Context, capability string, arguments json.RawMessage) (json.RawMessage, error)
	// Close 释放资源，允许重复调用；关闭的实例需要重新创建才能启动。
	Close() error
}

// Descriptor 描述扩展类型及其实现，不包含令牌或其他实例配置。
// 返回值是快照，调用方修改它不能改变扩展的行为。
type Descriptor struct {
	Type           string             `json:"type"`
	Implementation string             `json:"implementation"`
	Name           string             `json:"name"`
	Description    string             `json:"description,omitempty"`
	Capabilities   []Capability       `json:"capabilities"`
	Parameters     []ParameterField   `json:"parameters"`
	Information    []InformationField `json:"information"`
}

// ParameterField 由扩展声明创建/编辑表单，不依赖具体前端框架。
type ParameterField struct {
	Key                  string `json:"key"`
	Label                string `json:"label"`
	Kind                 string `json:"kind"` // text、secret、integer
	Required             bool   `json:"required"`
	Default              string `json:"default,omitempty"`
	Placeholder          string `json:"placeholder,omitempty"`
	Help                 string `json:"help,omitempty"`
	Pattern              string `json:"pattern,omitempty"`
	Minimum              int    `json:"minimum,omitempty"`
	Maximum              int    `json:"maximum,omitempty"`
	EditableWhileRunning bool   `json:"editableWhileRunning"`
}

// InformationField 总是在信息窗口展示，InList 额外在列表行显示纯文本。
type InformationField struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	InList bool   `json:"inList"`
}

// Capability 的名称在一个扩展实例内唯一，例如 send_message。
// 宿主需要同时指定实例 ID，避免多个扩展的同名能力互相覆盖。
// Parameters 是 JSON Schema；能力描述不自动成为模型工具。
type Capability struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type State string

const (
	StateStopped State = "stopped"
	StateRunning State = "running"
	StateClosed  State = "closed"
)

// Status 区分扩展是否运行和外部连接是否在线。
// 本地软件等没有连接概念的扩展将 Connected 留空。
// 平台账号在线状态等额外信息由平台自己的事件或能力表达。
type Status struct {
	State     State  `json:"state"`
	Connected *bool  `json:"connected,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// Event 是结构化的外部事件，不是模型消息。
// Type 使用扩展类型命名空间，例如 qq.message、bilibili.comment。
// Scope 标识实例内的资源/会话，例如 group:123；宿主应结合实例 ID 路由。
// ID 是源事件 ID，未知时留空；宿主不能假设 ID 在不同实例间唯一。
// Data 的结构由扩展类型定义，交付后实现不得再修改其内容。
type Event struct {
	ID    string          `json:"id,omitempty"`
	Type  string          `json:"type"`
	Scope string          `json:"scope,omitempty"`
	Time  time.Time       `json:"time"`
	Data  json.RawMessage `json:"data"`
}

var (
	ErrClosed                = errors.New("扩展已关闭")
	ErrNotStarted            = errors.New("扩展尚未启动")
	ErrAlreadyStarted        = errors.New("扩展已经启动")
	ErrUnsupportedCapability = errors.New("扩展不支持该能力")
	ErrInvalidArguments      = errors.New("扩展能力参数无效")
)
