package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

func TestCoreUIUsesSameBackendAndShutsDownOnce(t *testing.T) {
	b := &fakeBackend{state: platform.NativeStatus{Supported: true}}
	engine := testCore(t, b)
	if engine.GetStatus().Ready || b.initCalls != 0 {
		t.Fatal("构造阶段初始化了平台")
	}
	engine.Startup(context.Background())
	engine.Startup(context.Background())
	status := engine.GetStatus()
	if !status.Ready || status.Name != "CAF" || status.DataDir != engine.Paths().Data || b.initCalls != 1 {
		t.Fatal(status)
	}
	status, err := engine.LoadNativeLibrary()
	if err != nil || !status.Native.Loaded || b.loadCalls != 1 {
		t.Fatal(status, err)
	}
	if _, err := engine.BeginLogin(); err != nil || b.loginCalls != 1 {
		t.Fatal(err)
	}
	engine.Shutdown(context.Background())
	engine.Shutdown(context.Background())
	if b.closeCalls != 1 || !engine.GetStatus().Closed || engine.GetStatus().Ready {
		t.Fatal("关闭没有使用同一后端，或重复关闭了后端")
	}
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrClosed) {
		t.Fatal(err)
	}
}

func TestCoreUIReportsOperationErrorsAndClearsThemOnSuccess(t *testing.T) {
	b := &fakeBackend{loadErr: errors.New("模拟加载失败"), loginErr: platform.ErrLoginNotImplemented}
	engine := testCore(t, b)
	engine.Startup(context.Background())
	status, err := engine.LoadNativeLibrary()
	if !errors.Is(err, b.loadErr) || status.LastError != err.Error() {
		t.Fatal(status, err)
	}
	b.loadErr = nil
	status, err = engine.LoadNativeLibrary()
	if err != nil || status.LastError != "" {
		t.Fatal(status, err)
	}
	status, err = engine.BeginLogin()
	if !errors.Is(err, platform.ErrLoginNotImplemented) || status.LastError != err.Error() {
		t.Fatal(status, err)
	}
}

func TestGlobalPlatformIsTheSameCoreUsedByCallers(t *testing.T) {
	previous := core.Platform
	t.Cleanup(func() { core.Platform = previous })
	core.Platform = nil
	b := &fakeBackend{}
	engine := testCore(t, b)
	if core.Platform != nil {
		t.Fatal("创建独立实例不应偷偷替换全局入口")
	}
	// 模拟 main 的装配；页面方法和其他包访问的是同一个指针。
	core.Platform = engine
	core.Platform.Startup(context.Background())
	if core.Platform != engine || core.Platform.Paths() != engine.Paths() {
		t.Fatal("全局入口不是同一核心")
	}
	if _, err := core.Platform.LoadNativeLibrary(); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	if b.initCalls != 1 || b.loadCalls != 1 || b.loginCalls != 1 {
		t.Fatal("没有使用同一组平台引用")
	}
	core.Platform.Shutdown(context.Background())
	if !engine.GetStatus().Closed || b.closeCalls != 1 {
		t.Fatal("全局关闭没有作用于同一实例")
	}
}

func TestCoreRejectsTypedNilBackend(t *testing.T) {
	var backend *fakeBackend
	if _, err := core.NewCore(testPaths(t), backend); !errors.Is(err, platform.ErrBackendRequired) {
		t.Fatal("接口里的带类型 nil 没有被拒绝", err)
	}
}

func TestCoreZeroValueReportsMissingBackendWithoutPanicking(t *testing.T) {
	engine := &core.Core{}
	if engine.GetStatus().Ready || engine.State().Native.Loaded {
		t.Fatal("零值核心应处于未就绪状态")
	}
	engine.Startup(context.Background())
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrBackendRequired) {
		t.Fatal(err)
	}
}

func TestCoreStartupCanRetryAfterBackendFailure(t *testing.T) {
	failure := errors.New("模拟启动失败")
	b := &fakeBackend{initErr: failure}
	engine := testCore(t, b)
	engine.Startup(context.Background())
	if _, err := engine.BeginLogin(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	b.initErr = nil
	engine.Startup(context.Background())
	if !engine.GetStatus().Ready || engine.GetStatus().LastError != "" || b.initCalls != 2 {
		t.Fatal(engine.GetStatus())
	}
	if _, err := engine.BeginLogin(); err != nil {
		t.Fatal(err)
	}
}

func TestCoreStartupRejectsCanceledContextWithoutInitializingBackend(t *testing.T) {
	b := &fakeBackend{}
	engine := testCore(t, b)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine.Startup(ctx)
	if _, err := engine.BeginLogin(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if b.initCalls != 0 {
		t.Fatal("无效上下文仍然初始化了后端")
	}
}

// 基础后端刻意不实现 QQPlatform，验证 QQ 不是 Backend 的强制组成部分。
type basicBackend struct{ target *fakeBackend }

func (b *basicBackend) Init(p platform.Paths) error                { return b.target.Init(p) }
func (b *basicBackend) NativeStatus() platform.NativeStatus        { return b.target.NativeStatus() }
func (b *basicBackend) LoadNative() (platform.NativeStatus, error) { return b.target.LoadNative() }
func (b *basicBackend) Close() error                               { return b.target.Close() }

func TestCoreReportsMissingQQCapability(t *testing.T) {
	b := &fakeBackend{}
	engine := testCore(t, &basicBackend{target: b})
	engine.Startup(context.Background())
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrQQUnavailable) {
		t.Fatal(err)
	}
	if b.loginCalls != 0 {
		t.Fatal("未提供 QQ 接口却尝试了登录")
	}
}

// 在后端观察实际收到的上下文，确保关闭核心会取消后续任务。
type contextBackend struct {
	fakeBackend
	received context.Context
}

func (b *contextBackend) BeginLogin(ctx context.Context) error {
	b.received = ctx
	return b.fakeBackend.BeginLogin(ctx)
}

func TestCoreCloseCancelsItsLifecycleContext(t *testing.T) {
	b := &contextBackend{}
	engine := testCore(t, b)
	parent := context.Background()
	engine.Startup(parent)
	if _, err := engine.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	if b.received == nil || b.received.Err() != nil {
		t.Fatal("未传入有效生命周期上下文")
	}
	engine.Shutdown(parent)
	if !errors.Is(b.received.Err(), context.Canceled) || parent.Err() != nil {
		t.Fatal("应取消核心子上下文，不影响调用方")
	}
}
