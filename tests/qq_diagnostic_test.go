package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/caliniya/caf/backend/desktop"
)

func TestQQSignerValidatesActualSDKRequestWithoutRealService(t *testing.T) {
	var requestOK atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		requestOK.Store(err == nil && r.Method == "POST" && r.URL.Path == "/sign" &&
			r.Header.Get("Content-Type") == "application/json" && r.Header.Get("Authorization") == "Bearer SYNTHETIC_TOKEN" &&
			r.Header.Get("User-Agent") == "qq/3.2.19-39038" && body["command"] == "wtlogin.trans_emp" &&
			body["seq"] == float64(7) && body["body"] == "0102" && body["uin"] == float64(0) &&
			body["guid"] == "SYNTHETIC_GUID" && body["qua"] == "SYNTHETIC_QUA")
		_, _ = io.WriteString(w, `{"code":0,"value":{"sec_sign":"0102","sec_token":"","sec_extra":""}}`)
	}))
	defer server.Close()
	signer := desktop.NewQQSigner(desktop.QQConfig{SignServer: server.URL + "/sign", SignToken: "SYNTHETIC_TOKEN"})
	defer signer.Release()
	signer.SetAppInfo(auth.AppList["linux"]["3.2.19-39038"])
	response, err := signer.Sign("wtlogin.trans_emp", 7, []byte{1, 2}, 0, "SYNTHETIC_GUID", "SYNTHETIC_QUA")
	if err != nil || response == nil || len(response.Value.SecSign) != 2 || !requestOK.Load() {
		t.Fatal("签名请求格式或响应验证错误（不输出请求、响应或认证头）")
	}
}

func TestQQSignerRejectsInvalidResponsesWithoutLeakingBody(t *testing.T) {
	for _, tc := range []struct {
		name, response, code string
		status               int
	}{
		{"unauthorized", "SYNTHETIC_SECRET", "SIGN_HTTP_401", 401},
		{"forbidden", "SYNTHETIC_SECRET", "SIGN_HTTP_403", 403},
		{"not-found", "SYNTHETIC_SECRET", "SIGN_HTTP_404", 404},
		{"method", "SYNTHETIC_SECRET", "SIGN_HTTP_405", 405},
		{"limited", "SYNTHETIC_SECRET", "SIGN_HTTP_429", 429},
		{"server", "SYNTHETIC_SECRET", "SIGN_HTTP_502", 502},
		{"business-code", `{"code":1001,"message":"SYNTHETIC_SECRET","value":{"sec_sign":"01"}}`, "SIGN_REJECTED_1001", 200},
		{"empty-json", `{}`, "SIGN_EMPTY_SIGNATURE", 200},
		{"old-format", `{"value":{"sign":"01","token":"02","extra":"03"}}`, "SIGN_EMPTY_SIGNATURE", 200},
		{"html-page", `<html>SYNTHETIC_SECRET</html>`, "SIGN_RESPONSE_FORMAT", 200},
		{"hex", `{"value":{"sec_sign":"SYNTHETIC_SECRET"}}`, "SIGN_RESPONSE_FORMAT", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()
			signer := desktop.NewQQSigner(desktop.QQConfig{SignServer: server.URL + "/?token=SYNTHETIC_SECRET"})
			defer signer.Release()
			_, err := signer.Sign("wtlogin.trans_emp", 1, []byte{0}, 0, "", "")
			if err == nil || !strings.Contains(err.Error(), "["+tc.code+"]") {
				t.Fatal("未返回预期脱敏错误码", tc.code)
			}
			if strings.Contains(err.Error(), "SYNTHETIC_SECRET") || strings.Contains(err.Error(), server.URL) {
				t.Fatal("签名错误泄露响应正文或接口地址")
			}
		})
	}
}

func TestQQSignerDoesNotContactServiceForUnsignedCommand(t *testing.T) {
	// 如果发生任何请求，这个地址不可用；非签名命令必须直接跳过。
	signer := desktop.NewQQSigner(desktop.QQConfig{SignServer: "http://127.0.0.1:1"})
	defer signer.Release()
	response, err := signer.Sign("CAF.Test.Unsigned", 0, nil, 0, "", "")
	if err != nil || response != nil {
		t.Fatal("非签名命令不应向服务发起请求")
	}
}

func TestQQFailuresExposeStageAndSafeCause(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		err        error
	}{
		{"timeout", "NETWORK_TIMEOUT", context.DeadlineExceeded},
		{"dns", "NETWORK_DNS", &net.DNSError{Err: "SYNTHETIC_SECRET", Name: "SYNTHETIC_SECRET"}},
		{"refused", "NETWORK_REFUSED", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		{"packet", "QQ_PACKET_TIMEOUT", errors.New("Packet timed out")},
		{"qr-code", "QQ_QR_REJECTED_17", errors.New("err qr retcode 17")},
		{"unknown", "QQ_UNKNOWN", fmt.Errorf("SYNTHETIC_SECRET: %s", "http://invalid.example/?token=SYNTHETIC_SECRET")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, dir, _ := sdkCore(t, &testQQSession{fetchErr: tc.err})
			configureSDKTest(t, dir)
			if _, err := e.BeginLogin(); err != nil {
				t.Fatal("未受理假会话登录")
			}
			state := waitQQ(t, e, "error")
			if state.Stage != "fetch_qr" || state.ErrorCode != tc.code || !strings.Contains(state.Error, "获取二维码失败") {
				t.Fatal("阶段或错误码丢失", state.Stage, state.ErrorCode)
			}
			encoded, _ := json.Marshal(e.GetStatus())
			if strings.Contains(string(encoded), "SYNTHETIC_SECRET") || strings.Contains(string(encoded), "invalid.example") {
				t.Fatal("诊断泄露底层错误或签名配置")
			}
		})
	}
}

func TestQQConfigurationAndVersionFailureHaveDistinctStages(t *testing.T) {
	e, _, _ := sdkCore(t, &testQQSession{})
	if _, err := e.BeginLogin(); err == nil {
		t.Fatal("缺配置应失败")
	}
	state := e.GetStatus().QQ
	if state.Phase != "error" || state.Stage != "config" || state.ErrorCode != "QQ_CONFIG" {
		t.Fatal("缺配置不能显示为网络或扫码失败")
	}
	_, err := desktop.NewLagrangeSession(context.Background(), desktop.QQConfig{Protocol: "invalid"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "[QQ_VERSION]") {
		t.Fatal("未知协议版本应明确报错，不应误报网络异常")
	}
}
