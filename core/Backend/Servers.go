package Backend

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

func (s *Servers) Get[T Server](name string) (T, error) {
	var zero T
	if s == nil {
		return zero, fmt.Errorf("服务注册表尚未初始化")
	}
	s.mu.RLock()
	service, exists := s.services[name]
	s.mu.RUnlock()
	if !exists || nilServer(service) {
		return zero, fmt.Errorf("服务不存在：%s", name)
	}
	result, ok := service.(T)
	if !ok {
		return zero, fmt.Errorf("服务类型不匹配：%s", name)
	}
	return result, nil
}

// Servers 是服务实例的唯一存储。使用后不得复制，注册后不允许替换实例。
// Get 只获取服务，不隐式创建实例或调用生命周期方法。
type Servers struct {
	mu       sync.RWMutex
	services map[string]Server
}

func nilServer(service Server) bool {
	if service == nil {
		return true
	}
	v := reflect.ValueOf(service)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Register 保存服务；重复注册返回错误，避免运行期间替换服务及其互斥锁。
func (s *Servers) Register(name string, service Server) error {
	if s == nil {
		return fmt.Errorf("服务注册表尚未初始化")
	}
	if strings.TrimSpace(name) == "" || nilServer(service) {
		return fmt.Errorf("服务名称和实例不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.services == nil {
		s.services = make(map[string]Server)
	}
	if _, exists := s.services[name]; exists {
		return fmt.Errorf("服务已注册：%s", name)
	}
	s.services[name] = service
	return nil
}

// initDefaults 仅填充缺失的内置服务，可重复调用，不改变已注册的实例。
func (s *Servers) initDefaults(core *Core) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.services == nil {
		s.services = make(map[string]Server)
	}
	if _, exists := s.services[QQServerName]; !exists {
		s.services[QQServerName] = NewQQServer(core)
	}
	if _, exists := s.services[ChatServerName]; !exists {
		s.services[ChatServerName] = NewChatServer(core)
	}
	if _, exists := s.services[ToolApprovalServerName]; !exists {
		s.services[ToolApprovalServerName] = NewToolApprovalServer(core)
	}
}

type Server interface {
	Init(ctx context.Context) error
	Close(ctx context.Context) error
	Start(ctx context.Context) error
}
