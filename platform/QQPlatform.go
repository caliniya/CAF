package platform

import (
	"github.com/LagrangeDev/LagrangeGo/client"
)

type QQConfig struct {
	GUID          string `json:"guid"`
	DeviceName    string `json:"device_name"`
	SystemKernel  string `json:"system_kernel"`
	KernelVersion string `json:"kernel_version"`

	OS      string `json:"os"`
	Version string `json:"version"`

	SignSever string `json:"signServer"`

	QQ    uint32 `json:"qq"`
	Token string `json:"signToken"`
}

// qq平台接口
// qq平台就是一个接口,确保实现了必须的方法
type QQPlatform interface {
	GetQQClient() *client.QQClient

	//auth.LoadOrSaveDevice("path/to/deviceinfo.json")

	CreateQQPlatform()

	StartLogin() int

	//if return id not 0 ,then it is error
	LoadQQConfigFromFile(fi string) int
}
