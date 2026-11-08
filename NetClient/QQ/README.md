# QQ 客户端

`QQClient` 只提供账号状态、事件、发送消息和关闭客户端。聊天调度、会话记录与通知主人由上层处理。

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

发送消息使用内部 `APIImpl.QQMessage`：

```go
result, err := client.SendMessage(ctx, APIImpl.QQMessage{
    Type:    APIImpl.QQMessageGroup,
    GroupID: groupID,
    Segments: []APIImpl.QQMessageSegment{
        APIImpl.QQText(reply),
    },
})
```

`SendResult.MessageID` 是对端返回的消息 ID。取消、超时或断线后，如果 `SendError.MayHaveSent` 为 true，发送结果未知，不自动重发。数组格式发送保证正文里的 CQ 字样仍是普通文字。

`Events()` 由一个消费者持续读取，再由上层分发；网络读取与事件处理分开，模型耗时不会阻塞动作响应。事件队列满时断开连接，`Info().LastError` 和状态事件说明原因。好友申请、群申请只上报，不自动处理。

CAF 的 QQ 页面提供临时连接测试面板，可填写地址、路径和令牌后手动启动适配器，查看事件并发送私聊或群聊文本。切换页面后连接继续运行，退出应用时关闭。测试配置和日志只保存在内存中。
