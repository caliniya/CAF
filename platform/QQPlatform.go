package platform

import (
	"github.com/LagrangeDev/LagrangeGo/client"
	//"github.com/LagrangeDev/LagrangeGo/client/auth"
)

// qq平台接口
// qq平台就是一个接口,确保实现了必须的方法
type QQPlatform interface {
	GetQQClient() *client.QQClient

	//auth.LoadOrSaveDevice("path/to/deviceinfo.json")

	CreateQQPlatform()

	StartLogin() int
}
