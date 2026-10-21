package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	qqclient "github.com/LagrangeDev/LagrangeGo/client"
	"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/LagrangeDev/LagrangeGo/client/packets/wtlogin/qrcodestate"
	"github.com/LagrangeDev/LagrangeGo/client/sign"
	"github.com/LagrangeDev/LagrangeGo/message"
)

type lagrangeSession struct {
	client    *qqclient.QQClient
	directory string
	mu        sync.Mutex
	once      sync.Once
	stop      chan struct{}
}

// NewLagrangeSession 创建离线的 SDK 适配对象；只有调用登录方法才联网。
func NewLagrangeSession(ctx context.Context, cfg QQConfig, directory string) (QQSession, error) {
	if ctx == nil {
		return nil, errors.New("必须提供上下文")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	app := auth.AppList[cfg.Protocol][cfg.Version]
	if app == nil {
		return nil, errors.New("不支持的 QQ 协议版本")
	}
	devicePath := filepath.Join(directory, "device.json")
	data, err := os.ReadFile(devicePath)
	device := &auth.DeviceInfo{}
	if errors.Is(err, os.ErrNotExist) {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return nil, err
		}
		device = &auth.DeviceInfo{GUID: hex.EncodeToString(id), DeviceName: "CAF-" + hex.EncodeToString(id[:4]), SystemKernel: runtime.GOOS, KernelVersion: runtime.GOARCH}
		data, err = json.Marshal(device)
		if err != nil {
			return nil, err
		}
		if err := writeQQFile(devicePath, data); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if err := json.Unmarshal(data, device); err != nil {
		return nil, err
	}
	id, err := hex.DecodeString(device.GUID)
	if err != nil || len(id) != 16 || device.DeviceName == "" {
		return nil, errors.New("设备文件无效")
	}

	client := qqclient.NewClientEmpty()
	// SDK 日志和报文转储可能包含票据；试验后端不把它们转发到页面或普通日志。
	client.SetLogger(quietQQLogger{})
	client.UseVersion(app)
	client.UseDevice(device)
	client.UseSignProvider(sign.NewSigner(nil, cfg.SignToken, cfg.SignServer))
	data, err = os.ReadFile(filepath.Join(directory, "sig.bin"))
	if err == nil {
		sig, err := auth.UnmarshalSigInfo(data, true)
		if err != nil {
			client.Release()
			return nil, err
		}
		client.UseSig(sig)
	} else if !errors.Is(err, os.ErrNotExist) {
		client.Release()
		return nil, err
	}
	session := &lagrangeSession{client: client, directory: directory, stop: make(chan struct{})}
	// SDK 的方法没有 context 参数；取消时中断 TCP，使等待中的调用尽快结束。
	// HTTP 签名请求仍由 SDK 自身的超时控制，不宣称所有操作都能瞬间取消。
	go func() {
		select {
		case <-ctx.Done():
			client.Disconnect()
		case <-session.stop:
		}
	}()
	return session, nil
}

func (s *lagrangeSession) FastLogin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client.FastLogin()
}
func (s *lagrangeSession) FetchQRCode() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	png, _, err := s.client.FetchQRCodeDefault()
	return png, err
}
func (s *lagrangeSession) QueryQRCode() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.client.GetQRCodeResult()
	switch state {
	case qrcodestate.Confirmed:
		return "confirmed", err
	case qrcodestate.WaitingForScan:
		return "waiting", err
	case qrcodestate.WaitingForConfirm:
		return "scanned", err
	default:
		return "expired", err
	}
}
func (s *lagrangeSession) QRCodeLogin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.client.QRCodeLogin()
	if err != nil {
		return err
	}
	if result == nil || !result.Success {
		return errors.New("QQ 没有确认登录成功")
	}
	return nil
}
func (s *lagrangeSession) Online() bool    { return s.client.Online.Load() }
func (s *lagrangeSession) Account() uint32 { return s.client.Uin }
func (s *lagrangeSession) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.client.Sig().Marshal()
	if err != nil {
		return err
	}
	return writeQQFile(filepath.Join(s.directory, "sig.bin"), data)
}
func (s *lagrangeSession) SendGroupText(group uint32, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.client.Online.Load() {
		return errors.New("QQ 尚未上线")
	}
	result, err := s.client.SendGroupMessage(group, []message.IMessageElement{message.NewText(text)})
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("消息没有发送结果")
	}
	return nil
}
func (s *lagrangeSession) Close() {
	s.once.Do(func() {
		close(s.stop)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.client.Release()
	})
}

// quietQQLogger 不输出 SDK 的认证过程、报文和原始错误。
type quietQQLogger struct{}

func (quietQQLogger) Infof(string, ...any)        {}
func (quietQQLogger) Warningf(string, ...any)     {}
func (quietQQLogger) Errorf(string, ...any)       {}
func (quietQQLogger) Debugf(string, ...any)       {}
func (quietQQLogger) Dump([]byte, string, ...any) {}
