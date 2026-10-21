package desktop

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caliniya/caf/core/platform"
)

// DesktopQQ 持有一个 Go SDK 会话。网络登录在后台执行，不持有核心或后端的锁。
// 只有显式 BeginLogin 才读取配置并创建会话；构造、初始化和查询均不联网。
type DesktopQQ struct {
	backend *DesktopBackend
	mu      sync.Mutex
	factory QQSessionFactory
	state   platform.QQStatus
	session QQSession
	cancel  context.CancelFunc
	done    chan struct{}
	closed  bool
}

var _ platform.QQPlatform = (*DesktopQQ)(nil)
var _ platform.QQStatusProvider = (*DesktopQQ)(nil)
var _ platform.QQLoginControl = (*DesktopQQ)(nil)
var _ platform.QQMessenger = (*DesktopQQ)(nil)

func (b *DesktopBackend) QQ() platform.QQPlatform {
	if b == nil || b.qq == nil {
		return nil
	}
	return b.qq
}

func (q *DesktopQQ) QQStatus() platform.QQStatus {
	if q == nil || q.backend == nil {
		return platform.QQStatus{Phase: "unavailable"}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

func (q *DesktopQQ) BeginLogin(ctx context.Context) error {
	if q == nil || q.backend == nil {
		return platform.ErrQQUnavailable
	}
	q.backend.mu.Lock()
	err := q.backend.readyError()
	directory := filepath.Join(q.backend.paths.Data, "qq")
	q.backend.mu.Unlock()
	if err != nil {
		return err
	}
	if ctx == nil {
		return platform.ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return platform.ErrClosed
	}
	if q.done != nil {
		select {
		case <-q.done:
		default:
			return platform.ErrQQLoginInProgress
		}
	}
	cfg, err := loadQQConfig(directory)
	if err != nil {
		q.state.Error = err.Error()
		return err
	}
	worker, cancel := context.WithCancel(ctx)
	q.cancel = cancel
	q.done = make(chan struct{})
	q.state = platform.QQStatus{Provider: "LagrangeGo", Phase: "starting"}
	done := q.done
	go func() {
		// 失败后也释放父上下文中的子任务引用；不能等下次登录覆盖 cancel。
		defer cancel()
		q.login(worker, cfg, directory, done)
	}()
	return nil
}

func (q *DesktopQQ) publish(ctx context.Context, phase, qr, message string, account uint32) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || ctx.Err() != nil {
		return
	}
	q.state = platform.QQStatus{Provider: "LagrangeGo", Phase: phase, QRCode: qr, Error: message, Account: account}
}

func (q *DesktopQQ) login(ctx context.Context, cfg QQConfig, directory string, done chan struct{}) {
	qrPath := filepath.Join(directory, "qrcode.png")
	defer func() {
		// 本工作协程的异常只转换为通用错误，不把凭据或请求详情带到页面。
		if recover() != nil {
			q.publish(ctx, "error", "", "QQ SDK 发生异常，请检查配置后重试", 0)
		}
		_ = os.Remove(qrPath)
		q.mu.Lock()
		q.session = nil
		q.state.QRCode = ""
		if q.closed {
			q.state.Phase = "closed"
		} else if ctx.Err() != nil {
			q.state.Phase = "idle"
			q.state.Account = 0
			q.state.Error = ""
		}
		q.mu.Unlock()
		close(done)
	}()
	session, err := q.factory(ctx, cfg, directory)
	if err != nil || session == nil {
		if session != nil {
			session.Close()
		}
		q.publish(ctx, "error", "", "无法创建 QQ 会话，请检查协议版本、设备与凭据文件", 0)
		return
	}
	defer session.Close()
	q.mu.Lock()
	q.session = session
	q.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if session.FastLogin() != nil {
		if ctx.Err() != nil {
			return
		}
		png, err := session.FetchQRCode()
		if err != nil || len(png) == 0 || len(png) > 2*1024*1024 {
			q.publish(ctx, "error", "", "获取二维码失败，请检查网络、协议版本和签名服务", 0)
			return
		}
		if err := writeQQFile(qrPath, png); err != nil {
			q.publish(ctx, "error", "", "无法保存本机二维码文件", 0)
			return
		}
		qr := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		q.publish(ctx, "waiting", qr, "", 0)
		for {
			if ctx.Err() != nil {
				return
			}
			phase, err := session.QueryQRCode()
			if err != nil {
				q.publish(ctx, "error", "", "查询扫码状态失败，请重新发起登录", 0)
				return
			}
			if phase == "confirmed" {
				break
			}
			if phase != "waiting" && phase != "scanned" {
				q.publish(ctx, "error", "", "二维码已过期或被取消，请重新发起登录", 0)
				return
			}
			q.publish(ctx, phase, qr, "", 0)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
		q.publish(ctx, "logging_in", "", "", 0)
		if err := session.QRCodeLogin(); err != nil {
			q.publish(ctx, "error", "", "QQ 登录失败，请检查签名服务与协议版本", 0)
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	if !session.Online() {
		q.publish(ctx, "error", "", "SDK 未确认 QQ 上线，不能视为登录成功", 0)
		return
	}
	if err := session.Save(); err != nil {
		q.publish(ctx, "error", "", "无法保存登录凭据，本次会话已关闭", 0)
		return
	}
	_ = os.Remove(qrPath)
	q.publish(ctx, "online", "", "", session.Account())
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !session.Online() {
				q.publish(ctx, "error", "", "QQ 连接已断开，请重新登录", 0)
				return
			}
		}
	}
}

func (q *DesktopQQ) CancelLogin() error {
	if q == nil || q.backend == nil {
		return platform.ErrQQUnavailable
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return platform.ErrClosed
	}
	if q.cancel != nil {
		q.cancel()
	}
	return nil
}

func (q *DesktopQQ) close() {
	q.mu.Lock()
	q.closed = true
	q.state.Phase = "closed"
	q.state.QRCode = ""
	q.state.Account = 0
	cancel, done := q.cancel, q.done
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (q *DesktopQQ) SendGroupText(ctx context.Context, group uint32, text string) error {
	if q == nil || q.backend == nil {
		return platform.ErrQQUnavailable
	}
	if ctx == nil {
		return platform.ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if group == 0 || strings.TrimSpace(text) == "" || len([]rune(text)) > 4096 {
		return errors.New("群号必须有效，文本长度必须为 1 到 4096 字")
	}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return platform.ErrClosed
	}
	if q.state.Phase != "online" || q.session == nil {
		q.mu.Unlock()
		return platform.ErrQQNotOnline
	}
	session := q.session
	q.mu.Unlock()
	if err := session.SendGroupText(group, text); err != nil {
		return errors.New("群消息发送失败，请检查网络、群号和账号权限；不要自动重试以免重复发送")
	}
	return nil
}
