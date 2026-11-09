# QQ 客户端

`QQClient` 只提供账号状态、事件、发送消息和关闭客户端。聊天调度、会话记录与通知主人由上层处理。

## QQ 扩展

QQ 的扩展类型为 `qq`，OneBot 11 和 LagrangeGo 是具体实现。通用接口位于 [`Extension.go`](../Extension.go)，不依赖 QQ 或模型消息。QQ 专用 `Extension` 嵌入通用接口，并保留类型明确的 `Info()` 与 `SendMessage()`。

QQ 扩展、消息类型以及 OneBot 11 的协议类型、编解码和 WebSocket 实现统一位于 `core/Extension/QQ`，Go 导入路径为 `github.com/caliniya/caf/core/Extension/QQ`。

`NewReverseWSExtension` 创建 OneBot 11 的 QQ 扩展实现，提供 `Describe()`、`Status()`、`Start()`、`Events()`、`Invoke()`、`Close()`。`Events()` 输出通用事件，类型为 `qq.message` 等，`Data` 保留 `QQ.Event`；`Invoke(ctx, "send_message", arguments)` 使用 `QQMessage` 的 JSON 发送消息，返回 `SendResult` 的 JSON。扩展只接收事件、提供能力，Agent 决定会话路由及模型上下文。

QQ 客户端管理已使用 `QQ.Extension`，并推送带实例 ID 的 `extension:event`；原有 `qq:event` 保留。底层 `ReverseWSClient` 仍可单独使用，扩展实现独占底层事件流，不能再用第二个消费者读取同一客户端。

## OneBot 11 反向 WebSocket

`ReverseWSClient` 接受 OneBot 11 Universal 反向连接，事件和动作响应共用一条连接。可自行监听，也可作为 `http.Handler` 挂载到现有 HTTP 服务。

```go
client, err := QQ.NewReverseWSClient(QQ.ReverseWSConfig{
    Address:     "127.0.0.1:6701",
    Path:        "/onebot/v11/ws",
    AccessToken: token,
    SelfID:      botAccount,
})
if err != nil {
    return err
}
defer client.Close()
if err := client.Start(ctx); err != nil {
    return err
}
for event := range client.Events() {
    // 把消息、申请、通知和状态交给上层分发。
    handleEvent(event)
}
```

在 NapCat 或其他 OneBot 11 实现中配置反向 WebSocket，URL 为 `ws://<CAF 所在主机>:6701/onebot/v11/ws`，令牌与 `AccessToken` 一致。客户端握手应带 `X-Self-ID`，`X-Client-Role` 为 `Universal` 或省略。认证支持 `Authorization: Bearer ...` 和 `access_token` 查询参数。

NapCat 位于 WSL、CAF 位于 Windows 时，URL 使用 WSL 能访问的 Windows 地址；如果需要从非回环地址接入，可把监听地址改为 `0.0.0.0:6701`。路径和端口由上层配置。

`SelfID` 留空时，首次连接绑定机器人账号；同账号新连接替换旧连接。掉线保留客户端和事件流，等待对端再次反向连接。WebSocket Ping/Pong 检测失联，OneBot 心跳中的 `status.online` 单独表示 QQ 在线状态。

发送消息使用 QQ 扩展的 `QQ.QQMessage`：

```go
result, err := client.SendMessage(ctx, QQ.QQMessage{
    Type:    QQ.QQMessageGroup,
    GroupID: groupID,
    Segments: []QQ.QQMessageSegment{
        QQ.QQText(reply),
    },
})
```

`SendResult.MessageID` 是对端返回的消息 ID。取消、超时或断线后，如果 `SendError.MayHaveSent` 为 true，发送结果未知，不自动重发。数组格式发送保证正文里的 CQ 字样仍是普通文字。

`Events()` 由一个消费者持续读取，再由上层分发；网络读取与事件处理分开，模型耗时不会阻塞动作响应。事件队列满时断开连接，`Info().LastError` 和状态事件说明原因。好友申请、群申请只上报，不自动处理。

调用 `Start` 或挂载 HTTP handler 后开始接收连接。

左上角切换到 Ext 后，主界面列出应用管理的客户端。创建只填写名称和 ID，数据保存在应用数据目录的 `ext/clients/<ID>/client.json`，无需选择外部文件夹。ID 是固定的目录名称，限制为字母、数字、下划线及短横线，并拒绝重复 ID 和系统保留名称。删除客户端会关闭所属扩展，将整个内部目录移至 `ext/archive/clients/<ID>-<归档标识>`，释放 ID 供新客户端使用；复用 ID 不继承旧数据或扩展绑定。旧版删除留下的 `archived.json` 目录会在复用 ID 时自动迁至归档区。

每个客户端可以添加多个 QQ 扩展，每个扩展需要填写独立名称并选择适配器。同一客户端的扩展名称不能重复；一个适配器只绑定一个客户端。适配器创建窗口保留名称、Token、可选 QQ 号和可复制的连接地址，每个适配器从 6701 开始分配独立端口。启用扩展会启动监听，连接后显示「已连接」；停用或移除会关闭连接。应用启动时恢复已启用扩展的监听。Token、账号和适配器配置仍保存在 `qq/adapters.json`，已有适配器可以直接选择绑定。

当前 QQ 扩展只识别明确指令。名称为「助手」时，`/助手` 或 `/助手 help` 显示帮助，`/助手 status` 返回所属客户端状态；也支持开头引用及 @ 本账号。普通聊天、自身消息、@其他账号和其他名称前缀不会触发回复。回复由固定逻辑生成，不调用模型，不发起主动对话。指令在独立有界队列中处理，按事件 ID 去重，不自动重试发送结果未知的操作。

监听地址为 `0.0.0.0`，本机连接使用界面显示的 `127.0.0.1`，WSL 或远程客户端将其替换为 CAF 主机的可访问地址。运行时修改 Token 或 QQ 号需先停用扩展；删除适配器前先移除客户端中的绑定。
