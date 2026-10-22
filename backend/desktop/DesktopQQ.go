package desktopBackend

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/LagrangeDev/LagrangeGo/client"
	"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/LagrangeDev/LagrangeGo/client/sign"
	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/platform"
)

// qq平台在桌面的实现
var DesktopQQPlatform *DesktopQQ

type DesktopQQ struct {
	qqClient     *client.QQClient
	QQConfigFile string
	QQCfg        *platform.QQConfig
}

func (qq *DesktopQQ) StartLogin() int {
	return 0
}

func (qq *DesktopQQ) CreateQQPlatform() {
	core.Platform.QQPlatform = qq
	qq.QQConfigFile = filepath.Join(core.Platform.Paths.Data, "qq", "QQConfig.json")
	qq.qqClient = client.NewClientEmpty()
}

func (qq *DesktopQQ) GetQQClient() *client.QQClient {
	return qq.qqClient
}

func (qq *DesktopQQ) LoadQQConfigFromFile(fi string) int {
	data, err := os.ReadFile(qq.QQConfigFile)
	if err != nil {
		log.Printf("读取配置失败：%v", err)
		return 1
	}

	qq.QQCfg = &platform.QQConfig{}

	josnErr := json.Unmarshal(data, qq.QQCfg)

	qq.qqClient.UseVersion(auth.AppList[qq.QQCfg.OS][qq.QQCfg.Version])

	qq.qqClient.UseDevice(&auth.DeviceInfo{
		GUID:          qq.QQCfg.GUID,
		DeviceName:    qq.QQCfg.DeviceName,
		SystemKernel:  qq.QQCfg.SystemKernel,
		KernelVersion: qq.QQCfg.KernelVersion,
	})

	qq.qqClient.Uin = qq.QQCfg.QQ

	qq.qqClient.UseSignProvider(sign.NewSigner(log.Printf, qq.QQCfg.Token, qq.QQCfg.SignSever))

	if josnErr == nil {
		return 0
	}

	return 4
}
