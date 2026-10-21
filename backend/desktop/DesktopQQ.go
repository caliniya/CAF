package desktop

import (
	"context"

	"github.com/caliniya/caf/core/platform"
)

// DesktopQQ 是桌面端的 QQ 占位实现，与所属后端共用初始化和关闭状态。
// 当前不创建会话、不加载本地库，也不调用保留的原生绑定。
type DesktopQQ struct {
	backend *DesktopBackend
}

var _ platform.QQPlatform = (*DesktopQQ)(nil)

// QQ 返回同一个 QQ 实现，供入口注入核心；此方法不执行初始化或登录。
func (b *DesktopBackend) QQ() platform.QQPlatform {
	if b == nil || b.qq == nil {
		return nil
	}
	return b.qq
}

func (q *DesktopQQ) BeginLogin(ctx context.Context) error {
	if q == nil || q.backend == nil {
		return platform.ErrQQUnavailable
	}
	b := q.backend
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.readyError(); err != nil {
		return err
	}
	if ctx == nil {
		return platform.ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return platform.ErrQQIntegrationPaused
}
