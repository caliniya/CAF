package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

// 只实现 QQ 能力，不实现基础后端，验证核心确实使用单独注入的引用。
type fakeQQ struct {
	calls int
	err   error
}

func (q *fakeQQ) BeginLogin(context.Context) error {
	q.calls++
	return q.err
}

func TestCoreUsesExplicitQQImplementation(t *testing.T) {
	b := &fakeBackend{}
	qq := &fakeQQ{err: platform.ErrQQIntegrationPaused}
	engine, err := core.NewCore(testPaths(t), b, qq)
	if err != nil {
		t.Fatal(err)
	}
	if qq.calls != 0 || b.initCalls != 0 {
		t.Fatal("构造核心不应初始化后端或调用 QQ")
	}
	engine.Startup(context.Background())
	defer engine.Close()
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrQQIntegrationPaused) {
		t.Fatal(err)
	}
	if qq.calls != 1 || b.loginCalls != 0 || !engine.GetStatus().Ready {
		t.Fatal("核心没有使用单独注入的 QQ 引用，或暂停影响了核心就绪")
	}
}

func TestCoreDoesNotInferQQCapabilityFromBackend(t *testing.T) {
	b := &fakeBackend{}
	engine, err := core.NewCore(testPaths(t), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine.Startup(context.Background())
	defer engine.Close()
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrQQUnavailable) || b.loginCalls != 0 {
		t.Fatal("没有提供 QQ 引用时不能自动把基础后端当成 QQ 实现", err)
	}
}

func TestCoreTreatsTypedNilQQAsUnavailable(t *testing.T) {
	var qq *fakeQQ
	engine, err := core.NewCore(testPaths(t), &fakeBackend{}, qq)
	if err != nil {
		t.Fatal(err)
	}
	engine.Startup(context.Background())
	defer engine.Close()
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrQQUnavailable) {
		t.Fatal("带类型 nil 应视为未提供 QQ，而不是调用并崩溃", err)
	}
}

func TestDesktopProvidesOneSeparateQQPlaceholder(t *testing.T) {
	b := desktop.New()
	qq := b.QQ()
	if qq == nil || b.QQ() != qq {
		t.Fatal("桌面后端应稳定提供同一个 QQ 引用")
	}
	if _, ok := qq.(*desktop.DesktopQQ); !ok {
		t.Fatal("QQ 能力应由 DesktopQQ 实现")
	}
	if _, ok := any(b).(platform.QQPlatform); ok {
		t.Fatal("基础后端不应再次兼任 QQ 实现")
	}
	if b.NativeStatus().Loaded {
		t.Fatal("取得 QQ 引用不应加载原生库")
	}
}

func TestUnboundDesktopQQReportsUnavailable(t *testing.T) {
	var missing *desktop.DesktopQQ
	for _, qq := range []platform.QQPlatform{missing, &desktop.DesktopQQ{}} {
		if err := qq.BeginLogin(context.Background()); !errors.Is(err, platform.ErrQQUnavailable) {
			t.Fatal("未绑定后端的 QQ 占位不能崩溃或假报成功", err)
		}
	}
	var backend *desktop.DesktopBackend
	if backend.QQ() != nil || (&desktop.DesktopBackend{}).QQ() != nil {
		t.Fatal("未构造的后端不应提供无效 QQ 引用")
	}
}
