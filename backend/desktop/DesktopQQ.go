package desktopBackend

import (
	"github.com/LagrangeDev/LagrangeGo/client"
	//"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/caliniya/caf/core"
)

// qq平台在桌面的实现
var DesktopQQPlatform *DesktopQQ

type DesktopQQ struct {
	qqClient     *client.QQClient
	QQConfigFile string
}

func (qq *DesktopQQ) StartLogin() int {
	return 0
}

func (qq *DesktopQQ) CreateQQPlatform() {
	core.Platform.QQPlatform = qq
}

func (qq *DesktopQQ) GetQQClient() *client.QQClient {
	return qq.qqClient
}

func (qq *DesktopQQ) LoadQQConfigFromFile(fi string) {

}
