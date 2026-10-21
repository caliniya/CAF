package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"syscall"
)

// qqDiagnosticError 只保存可以显示的错误码和中文说明，不保存 URL、响应正文或认证数据。
type qqDiagnosticError struct {
	code    string
	message string
}

func (e *qqDiagnosticError) Error() string { return "[" + e.code + "] " + e.message }

func qqDiagnostic(code, message string) error {
	return &qqDiagnosticError{code: code, message: message}
}

var qrReturnCode = regexp.MustCompile(`^err qr retcode ([0-9]{1,3})$`)

// describeQQError 使用白名单生成诊断，不把任意 SDK error.Error() 拼到页面或日志。
func describeQQError(err error) (string, string) {
	var diagnostic *qqDiagnosticError
	if errors.As(err, &diagnostic) {
		return diagnostic.code, diagnostic.message
	}
	var dns *net.DNSError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "CANCELED", "操作已取消"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &network) && network.Timeout():
		return "NETWORK_TIMEOUT", "网络请求超时；检查当前网络和服务是否可达"
	case errors.As(err, &dns):
		return "NETWORK_DNS", "域名解析失败；检查 DNS 和当前网络"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "NETWORK_REFUSED", "连接被拒绝；检查服务进程、监听地址与端口"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return "NETWORK_UNREACHABLE", "网络或目标主机不可达；检查路由和防火墙"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed), errors.Is(err, syscall.ECONNRESET):
		return "NETWORK_CLOSED", "连接已关闭或返回数据不完整"
	case errors.Is(err, os.ErrPermission):
		return "FILE_PERMISSION", "无法读写本机文件；检查数据目录权限"
	}
	// 以下字符串是固定版本 SDK 的固定错误，不能泛化为输出底层原始文本。
	if err != nil {
		switch err.Error() {
		case "Packet timed out":
			return "QQ_PACKET_TIMEOUT", "等待 QQ 服务器响应超时；这不等同于签名服务器超时"
		case "All servers are unreachable":
			return "QQ_SERVER_UNREACHABLE", "QQ 服务器不可达；检查 TCP 网络与防火墙"
		}
		if match := qrReturnCode.FindStringSubmatch(err.Error()); match != nil {
			return "QQ_QR_REJECTED_" + match[1], fmt.Sprintf("QQ 服务器拒绝二维码请求（返回码 %s）；核对协议版本及签名服务兼容性", match[1])
		}
	}
	return "QQ_UNKNOWN", "底层原因暂未分类；请提供此失败阶段和错误码，不要公开配置、票据或原始报文"
}
