package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/caliniya/caf/backend/desktop"
)

func TestQQSignerUsesBoundAccountBeforeConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		configured, actual, want uint32
		mismatch                 bool
	}{
		{"automatic-before-scan", 0, 0, 0, false},
		{"automatic-after-scan", 0, 987654, 987654, false},
		{"bound-before-scan", 123456, 0, 123456, false},
		{"bound-matching-scan", 123456, 123456, 123456, false},
		{"bound-wrong-scan", 123456, 987654, 0, true},
		{"sdk-upper-bound", 4294967295, 0, 4294967295, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			var received atomic.Uint32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var body struct {
					Uin uint32 `json:"uin"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					w.WriteHeader(400)
					return
				}
				received.Store(body.Uin)
				// 模拟必须携带已绑定账号的服务；不正确的请求会被拒绝为 403。
				if tc.configured != 0 && body.Uin != tc.configured {
					w.WriteHeader(403)
					return
				}
				_, _ = io.WriteString(w, `{"code":0,"value":{"sec_sign":"01"}}`)
			}))
			defer server.Close()
			signer := desktop.NewQQSigner(desktop.QQConfig{QQ: tc.configured, SignServer: server.URL})
			defer signer.Release()
			response, err := signer.Sign("wtlogin.trans_emp", 1, []byte{0}, tc.actual, "", "")
			if tc.mismatch {
				if err == nil || !strings.Contains(err.Error(), "QQ_ACCOUNT_MISMATCH") || response != nil || requests.Load() != 0 {
					t.Fatal("真实账号不匹配时必须在请求签名前拒绝，不能伪装成绑定账号")
				}
				return
			}
			if err != nil || response == nil || requests.Load() != 1 || received.Load() != tc.want {
				t.Fatal("签名请求未使用预期账号（不输出请求内容）")
			}
		})
	}
}

// setQQAccountTest 只修改测试临时目录里的虚构配置，不读取用户的本机配置。
func setQQAccountTest(t *testing.T, directory string, account any) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{
		"qq": account, "signServer": "http://127.0.0.1:1", "signToken": "SYNTHETIC_PRIVATE_TEST",
		"protocol": "linux", "version": "3.2.19-39038",
	})
	if err := os.WriteFile(filepath.Join(directory, "config.json"), data, 0600); err != nil {
		t.Fatal("无法写入测试配置")
	}
}

func TestQQConfigIncludesAccountAndRejectsInvalidNumbers(t *testing.T) {
	e, dir, count := sdkCore(t, &testQQSession{})
	data, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var template map[string]any
	if json.Unmarshal(data, &template) != nil || template["qq"] != float64(0) {
		t.Fatal("新配置模板必须包含 qq: 0")
	}
	for _, value := range []any{-1, uint64(4294967296), 1.5, "123456", true} {
		setQQAccountTest(t, dir, value)
		if _, err := e.BeginLogin(); err == nil || !strings.Contains(err.Error(), "qq 必须是") {
			t.Fatal("非法 qq 必须在配置阶段明确拒绝")
		}
		if count.Load() != 0 || e.GetStatus().QQ.Stage != "config" {
			t.Fatal("非法账号不能启动 SDK 会话")
		}
	}
}

func TestQQConfiguredAccountCanCompleteMatchingFakeLogin(t *testing.T) {
	s := &testQQSession{}
	e, dir, _ := sdkCore(t, s)
	setQQAccountTest(t, dir, uint32(123456))
	if _, err := e.BeginLogin(); err != nil {
		t.Fatal("匹配的虚构账号未受理")
	}
	state := waitQQ(t, e, "online")
	if state.Account != 123456 || s.logins.Load() != 1 || s.saves.Load() != 1 || s.sends.Load() != 0 {
		t.Fatal("匹配账号的登录流程异常或自动发送了消息")
	}
}

func TestQQOldConfigWithoutAccountRemainsUsableAndUnmodified(t *testing.T) {
	e, dir, _ := sdkCore(t, &testQQSession{})
	data := []byte(`{"signServer":"http://127.0.0.1:1","protocol":"linux","version":"3.2.19-39038"}`)
	path := filepath.Join(dir, "config.json")
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("无法写入旧格式测试配置")
	}
	if _, err := e.BeginLogin(); err != nil {
		t.Fatal("未填写 qq 的旧配置仍应受理")
	}
	waitQQ(t, e, "online")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, data) {
		t.Fatal("不能自动覆盖用户已有的签名配置")
	}
}

func TestQQRejectsOtherAccountBeforeLoginOrCredentialSave(t *testing.T) {
	for _, fast := range []bool{false, true} {
		s := &testQQSession{fast: fast}
		e, dir, _ := sdkCore(t, s)
		setQQAccountTest(t, dir, uint32(654321))
		if _, err := e.BeginLogin(); err != nil {
			t.Fatal("未受理虚构登录")
		}
		state := waitQQ(t, e, "error")
		if state.ErrorCode != "QQ_ACCOUNT_MISMATCH" || state.Account != 0 || state.QRCode != "" {
			t.Fatal("不能把错误账号或配置账号报告为已上线")
		}
		if err := e.Close(); err != nil {
			t.Fatal("无法关闭测试核心")
		}
		if s.logins.Load() != 0 || s.saves.Load() != 0 || s.sends.Load() != 0 || s.closes.Load() != 1 {
			t.Fatal("不匹配账号不应继续扫码登录、保存凭据或发送消息")
		}
	}
}

func TestRealSDKRejectsOtherAccountCacheOfflineAndPreservesIt(t *testing.T) {
	dir := t.TempDir()
	sig := auth.SigInfo{Uin: 123456, D2: []byte("SYNTHETIC_NOT_A_REAL_TICKET")}
	data, err := sig.Marshal()
	if err != nil {
		t.Fatal("无法构造虚构缓存")
	}
	path := filepath.Join(dir, "sig.bin")
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("无法写入虚构缓存")
	}
	cfg := desktop.QQConfig{QQ: 654321, Protocol: "linux", Version: "3.2.19-39038", SignServer: "http://127.0.0.1:1"}
	session, err := desktop.NewLagrangeSession(context.Background(), cfg, dir)
	if err == nil || !strings.Contains(err.Error(), "QQ_CACHE_ACCOUNT_MISMATCH") || session != nil {
		if session != nil {
			session.Close()
		}
		t.Fatal("其他账号的缓存必须在联网前拒绝")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, data) {
		t.Fatal("不匹配的旧凭据不能被删除或覆盖")
	}
	cfg.QQ = 123456
	session, err = desktop.NewLagrangeSession(context.Background(), cfg, dir)
	if err != nil || session == nil {
		t.Fatal("相同账号的缓存应该可以离线加载")
	}
	defer session.Close()
	if session.Online() {
		t.Fatal("加载缓存不能自动联网或报告上线")
	}
}

func TestRealSDKConfiguredQQDoesNotSetAuthenticatedIdentity(t *testing.T) {
	cfg := desktop.QQConfig{QQ: 123456, Protocol: "linux", Version: "3.2.19-39038", SignServer: "http://127.0.0.1:1"}
	session, err := desktop.NewLagrangeSession(context.Background(), cfg, t.TempDir())
	if err != nil {
		t.Fatal("无法创建离线 SDK 会话")
	}
	defer session.Close()
	if session.Account() != 0 || session.Online() {
		t.Fatal("预设签名账号不能修改 QQ 客户端身份或假报上线")
	}
}
