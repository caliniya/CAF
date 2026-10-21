package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

type testQQSession struct {
	ctx         context.Context
	online      atomic.Bool
	fast        bool
	wait        bool
	unconfirmed bool
	fetchErr    error
	sends       atomic.Int32
	fetches     atomic.Int32
	logins      atomic.Int32
	saves       atomic.Int32
	closes      atomic.Int32
	mu          sync.Mutex
	text        string
	group       uint32
}

func (s *testQQSession) FastLogin() error {
	if s.fast {
		s.online.Store(true)
		return nil
	}
	return errors.New("测试会话没有缓存")
}
func (s *testQQSession) FetchQRCode() ([]byte, error) {
	s.fetches.Add(1)
	return []byte("测试二维码字节"), s.fetchErr
}
func (s *testQQSession) QueryQRCode() (string, error) {
	if s.wait {
		<-s.ctx.Done()
		return "", s.ctx.Err()
	}
	return "confirmed", nil
}
func (s *testQQSession) QRCodeLogin() error {
	s.logins.Add(1)
	s.online.Store(!s.unconfirmed)
	return nil
}
func (s *testQQSession) Online() bool    { return s.online.Load() }
func (s *testQQSession) Account() uint32 { return 123456 }
func (s *testQQSession) Save() error     { s.saves.Add(1); return nil }
func (s *testQQSession) SendGroupText(group uint32, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.group = group
	s.text = text
	s.sends.Add(1)
	return nil
}
func (s *testQQSession) Close() { s.closes.Add(1); s.online.Store(false) }

func sdkCore(t *testing.T, session *testQQSession) (*core.Core, string, *atomic.Int32) {
	t.Helper()
	var factories atomic.Int32
	b := desktop.NewWithQQFactory(func(ctx context.Context, _ desktop.QQConfig, _ string) (desktop.QQSession, error) {
		factories.Add(1)
		session.ctx = ctx
		return session, nil
	})
	paths := testPaths(t)
	engine, err := core.NewCore(paths, b, b.QQ())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
	return engine, filepath.Join(paths.Data, "qq"), &factories
}
func configureSDKTest(t *testing.T, directory string) {
	t.Helper()
	// 这是纯测试占位配置；假会话从不访问这个地址。
	cfg := desktop.QQConfig{SignServer: "http://127.0.0.1:1", SignToken: "SYNTHETIC_PRIVATE_TEST", Protocol: "linux", Version: "3.2.19-39038"}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(directory, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func waitQQ(t *testing.T, engine *core.Core, phase string) platform.QQStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state := engine.GetStatus().QQ
		if state.Phase == phase {
			return state
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("QQ 未进入预期状态 %s（不输出可能包含二维码的完整状态）", phase)
	return platform.QQStatus{}
}
func TestSDKInitializationIsOfflineAndMissingConfigIsExplicit(t *testing.T) {
	session := &testQQSession{}
	engine, directory, count := sdkCore(t, session)
	if count.Load() != 0 || engine.GetStatus().QQ.Provider != "LagrangeGo" || !engine.GetStatus().Ready {
		t.Fatal("初始化不应建立 QQ 会话")
	}
	data, err := os.ReadFile(filepath.Join(directory, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg desktop.QQConfig
	if json.Unmarshal(data, &cfg) != nil || cfg.SignServer != "" || cfg.SignToken != "" {
		t.Fatal("默认配置必须为空白且不包含凭据")
	}
	if _, err := engine.BeginLogin(); !errors.Is(err, platform.ErrQQConfigRequired) {
		t.Fatal(err)
	}
	if count.Load() != 0 || !engine.GetStatus().Ready {
		t.Fatal("缺少签名配置不能触发联网或影响核心初始化")
	}
}
func TestSDKQRCodeLoginAndExplicitGroupText(t *testing.T) {
	session := &testQQSession{}
	engine, dir, _ := sdkCore(t, session)
	configureSDKTest(t, dir)
	if _, err := engine.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	status := waitQQ(t, engine, "online")
	if status.Account != 123456 || status.QRCode != "" || session.fetches.Load() != 1 || session.logins.Load() != 1 || session.saves.Load() != 1 {
		t.Fatal("二维码登录或凭据保存流程错误")
	}
	if session.sends.Load() != 0 {
		t.Fatal("登录后不能自动发送消息")
	}
	if _, err := engine.SendGroupText(123, "测试消息"); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	correct := session.group == 123 && session.text == "测试消息"
	session.mu.Unlock()
	if !correct || session.sends.Load() != 1 {
		t.Fatal("显式发送没有使用当前会话")
	}
	if _, err := os.Stat(filepath.Join(dir, "qrcode.png")); !os.IsNotExist(err) {
		t.Fatal("上线后应清除短期二维码")
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if session.closes.Load() != 1 || engine.GetStatus().QQ.Phase != "closed" {
		t.Fatal("关闭没有释放同一个 SDK 会话")
	}
}
func TestSDKFastLoginSkipsQRCode(t *testing.T) {
	s := &testQQSession{fast: true}
	e, dir, _ := sdkCore(t, s)
	configureSDKTest(t, dir)
	if _, err := e.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	waitQQ(t, e, "online")
	if s.fetches.Load() != 0 || s.logins.Load() != 0 || s.saves.Load() != 1 {
		t.Fatal("缓存登录不应额外获取二维码")
	}
}
func TestSDKLoginCancellationAndDuplicateProtection(t *testing.T) {
	s := &testQQSession{wait: true}
	e, dir, count := sdkCore(t, s)
	configureSDKTest(t, dir)
	if _, err := e.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	waitQQ(t, e, "waiting")
	if _, err := e.BeginLogin(); !errors.Is(err, platform.ErrQQLoginInProgress) {
		t.Fatal(err)
	}
	if _, err := e.SendGroupText(123, "不能发送"); !errors.Is(err, platform.ErrQQNotOnline) {
		t.Fatal(err)
	}
	if _, err := e.CancelLogin(); err != nil {
		t.Fatal(err)
	}
	waitQQ(t, e, "idle")
	if count.Load() != 1 || s.closes.Load() != 1 || e.GetStatus().QQ.QRCode != "" {
		t.Fatal("取消应清理二维码并关闭会话")
	}
}
func TestSDKRawErrorsAndConfigNeverReachUI(t *testing.T) {
	s := &testQQSession{fetchErr: errors.New("https://invalid.example/?token=SYNTHETIC_PRIVATE_TEST")}
	e, dir, _ := sdkCore(t, s)
	configureSDKTest(t, dir)
	if _, err := e.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	waitQQ(t, e, "error")
	select {
	case <-s.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("失败后没有释放登录子上下文")
	}
	data, _ := json.Marshal(e.GetStatus())
	if bytes.Contains(data, []byte("SYNTHETIC_PRIVATE_TEST")) || bytes.Contains(data, []byte("signToken")) || bytes.Contains(data, []byte("invalid.example")) {
		t.Fatal("页面状态泄露了签名配置或 SDK 原始错误")
	}
}
func TestSDKDoesNotTreatNilErrorAsOnline(t *testing.T) {
	s := &testQQSession{unconfirmed: true}
	e, dir, _ := sdkCore(t, s)
	configureSDKTest(t, dir)
	if _, err := e.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	waitQQ(t, e, "error")
	if s.saves.Load() != 0 || e.GetStatus().QQ.Account != 0 {
		t.Fatal("SDK 未确认上线不能假报成功或保存凭据")
	}
}
func TestSDKConfigRejectsUnsafeOrMalformedAddresses(t *testing.T) {
	for _, raw := range []string{`{`, `{"signServer":"file:///tmp/sign"}`, `{"signServer":"http://user:pass@localhost"}`} {
		e, dir, count := sdkCore(t, &testQQSession{})
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.BeginLogin(); err == nil || count.Load() != 0 {
			t.Fatal("无效配置不能建立 SDK 会话")
		}
	}
}
func TestRealSDKFactoryCreatesStablePrivateDeviceWithoutLogin(t *testing.T) {
	cfg := desktop.QQConfig{SignServer: "http://127.0.0.1:1", Protocol: "linux", Version: "3.2.19-39038"}
	dir := t.TempDir()
	session, err := desktop.NewLagrangeSession(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal("创建 SDK 对象失败（隐藏底层详情）")
	}
	if session.Online() || session.Account() != 0 {
		t.Fatal("创建对象不能自动登录")
	}
	first, err := os.ReadFile(filepath.Join(dir, "device.json"))
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
	again, err := desktop.NewLagrangeSession(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal("无法复用设备")
	}
	again.Close()
	second, _ := os.ReadFile(filepath.Join(dir, "device.json"))
	if !bytes.Equal(first, second) {
		t.Fatal("设备标识不应在每次创建对象时重置")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "device.json"))
		if info.Mode().Perm() != 0600 {
			t.Fatal("设备文件权限应为 0600")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sig.bin")); !os.IsNotExist(err) {
		t.Fatal("未登录不能创建认证凭据")
	}
}
func TestRealSDKFactoryPreservesInvalidDeviceAndChecksVersion(t *testing.T) {
	cfg := desktop.QQConfig{Protocol: "linux", Version: "not-a-version"}
	if _, err := desktop.NewLagrangeSession(context.Background(), cfg, t.TempDir()); err == nil {
		t.Fatal("非法协议版本没有拒绝")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "device.json")
	original := []byte("do not overwrite")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Version = "3.2.19-39038"
	if _, err := desktop.NewLagrangeSession(context.Background(), cfg, dir); err == nil {
		t.Fatal("损坏设备文件不应被悄悄重置")
	}
	after, _ := os.ReadFile(path)
	if strings.TrimSpace(string(after)) != string(original) {
		t.Fatal("已有设备文件被覆盖")
	}
}
