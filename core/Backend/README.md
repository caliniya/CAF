# 后端 API

桌面应用调用的配置、聊天、工作区和扩展管理接口统一放在本包。

Go 导入路径为 `github.com/caliniya/caf/core/Backend`；`Core` 是 API 实例，`Platform` 由 `Init()` 初始化并交给桌面宿主绑定。API 文件共用实例的私有状态，因此 `Core.go`、`Config.go` 和 `ID.go` 也保存在本目录。

| 文件 | 职责 |
| --- | --- |
| `Backend.go` | 配置管理、会话及工具执行接口 |
| `ChatAPI.go` | 模型上下文、消息流和事件推送 |
| `WorkspaceAPI.go` | 工作区管理 |
| `ExtensionClientAPI.go` | 扩展客户端管理及内部数据目录 |
| `ExtensionUIAPI.go` | 扩展类型描述、实例和参数管理 |
| `QQState.go` | 集中保存 QQ 监听配置、适配器配置、运行状态及互斥锁 |
| `QQAdapterAPI.go` | Onebot11QQ 实例的配置与生命周期 |
| `QQCommandAPI.go` | Onebot11QQ 指令分发 |

Wails JavaScript 绑定地址为 `window.go.Backend.Core`，前端由 `src/api.ts` 统一调用。接口名称、参数、返回 JSON 和持久化数据格式保持原有约定。

`Core.QQ` 是可直接使用零值的 `QQState`，监听主机可在使用前通过 `Core.QQ.ListenHost` 设置。使用后不要整体复制该结构体，以免复制内部互斥锁。
