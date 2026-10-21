package desktop

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/caliniya/caf/core/platform"
)

// QQConfig 只从本地文件读取，不绑定给 Wails，也不写入日志。
// Protocol 指模拟的 QQ 协议平台，不是运行 CAF 的操作系统。
type QQConfig struct {
	SignServer string `json:"signServer"`
	SignToken  string `json:"signToken"`
	Protocol   string `json:"protocol"`
	Version    string `json:"version"`
}

func defaultQQConfig() QQConfig {
	return QQConfig{Protocol: "linux", Version: "3.2.19-39038"}
}

func prepareQQDirectory(data string) error {
	directory := filepath.Join(data, "qq")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	// 只创建空白模板，不覆盖用户已有的配置、设备或凭据。
	file, err := os.OpenFile(filepath.Join(directory, "config.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(defaultQQConfig())
}

func loadQQConfig(directory string) (QQConfig, error) {
	cfg := defaultQQConfig()
	data, err := os.ReadFile(filepath.Join(directory, "config.json"))
	if err != nil {
		return cfg, platform.ErrQQConfigRequired
	}
	if json.Unmarshal(data, &cfg) != nil {
		return cfg, errors.New("QQ 配置不是有效 JSON，请检查 qq/config.json")
	}
	if cfg.SignServer == "" {
		return cfg, platform.ErrQQConfigRequired
	}
	u, err := url.Parse(cfg.SignServer)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return cfg, errors.New("签名地址必须是有效的 HTTP/HTTPS 地址；认证令牌请填写 signToken")
	}
	return cfg, nil
}

// writeQQFile 用临时文件替换凭据，避免进程中断留下半份 JSON 或 sig.bin。
func writeQQFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".qq-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
