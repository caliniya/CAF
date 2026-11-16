package Backend

import (
	"context"
	"fmt"
)

func (s *Servers) Get[T Server](name string) (T, error) {
	var zero T
	if s == nil {
		return zero, fmt.Errorf("服务注册表尚未初始化")
	}
	service, exists := (*s)[name]
	if !exists || service == nil {
		return zero, fmt.Errorf("服务不存在：%s", name)
	}
	result, ok := service.(T)
	if !ok {
		return zero, fmt.Errorf("服务类型不匹配：%s", name)
	}
	return result, nil
}

type Servers map[string]Server

type Server interface {
	Init(ctx context.Context) error
	Close(ctx context.Context) error
	Start(ctx context.Context) error
}
