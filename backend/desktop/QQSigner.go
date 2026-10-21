package desktop

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/LagrangeDev/LagrangeGo/client/sign"
)

type checkedQQSigner struct {
	provider sign.Provider
	account  uint32
}

func NewQQSigner(cfg QQConfig) sign.Provider {
	return &checkedQQSigner{provider: sign.NewSigner(nil, cfg.SignToken, cfg.SignServer), account: cfg.QQ}
}

func (s *checkedQQSigner) AddRequestHeader(header map[string]string) {
	s.provider.AddRequestHeader(header)
}
func (s *checkedQQSigner) SetAppInfo(app *auth.AppInfo) { s.provider.SetAppInfo(app) }
func (s *checkedQQSigner) Release()                     { s.provider.Release() }

var signHTTPStatus = regexp.MustCompile(`^unexpected status code: ([1-5][0-9]{2})$`)

func (s *checkedQQSigner) Sign(command string, seq uint32, body []byte, uin uint32, guid, qua string) (*sign.Response, error) {
	if s.account != 0 && sign.ContainSignPKG(command) {
		// 扫码确认前 SDK 的 uin 是 0；仅向签名接口补充绑定账号，不修改 QQ 客户端身份。
		if uin == 0 {
			uin = s.account
		} else if err := validateQQAccount(s.account, uin); err != nil {
			// 实际账号已经明确时不能继续拿另一个绑定账号为其签名。
			return nil, err
		}
	}
	response, err := s.provider.Sign(command, seq, body, uin, guid, qua)
	if err != nil {
		if match := signHTTPStatus.FindStringSubmatch(err.Error()); match != nil {
			hint := "核对服务状态与完整接口地址"
			switch match[1] {
			case "401", "403":
				hint = "认证或权限失败；在本机检查 signToken、qq 绑定和服务授权"
			case "404", "405":
				hint = "接口路径或 POST 方法不匹配；不要只填写服务首页"
			case "429":
				hint = "请求受到限流；稍后手动重试"
			}
			return nil, qqDiagnostic("SIGN_HTTP_"+match[1], "签名服务返回 HTTP "+match[1]+"；"+hint)
		}
		var syntax *json.SyntaxError
		var field *json.UnmarshalTypeError
		var invalidHex hex.InvalidByteError
		if errors.As(err, &syntax) || errors.As(err, &field) || errors.As(err, &invalidHex) || errors.Is(err, hex.ErrLength) {
			return nil, qqDiagnostic("SIGN_RESPONSE_FORMAT", "签名响应不是预期的 JSON/十六进制格式；核对接口而不是服务首页")
		}
		code, detail := describeQQError(err)
		return nil, qqDiagnostic("SIGN_"+code, "签名请求失败："+detail)
	}
	if !sign.ContainSignPKG(command) {
		return response, nil
	}
	if response == nil {
		return nil, qqDiagnostic("SIGN_EMPTY_RESPONSE", "签名服务没有返回签名结果")
	}
	if response.Code != 0 {
		// 服务端 message 可能包含请求数据或认证链接，只保留数值 code。
		return nil, qqDiagnostic(fmt.Sprintf("SIGN_REJECTED_%d", response.Code), "签名服务拒绝了请求；核对接口、令牌权限与协议版本")
	}
	if len(response.Value.SecSign) == 0 {
		return nil, qqDiagnostic("SIGN_EMPTY_SIGNATURE", "签名响应缺少 value.sec_sign；HTTP 200 不代表协议兼容或签名成功")
	}
	return response, nil
}
