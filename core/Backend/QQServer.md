# QQ 服务

`QQServer` 实现 `Server` 的 `Init(ctx)`、`Start(ctx)` 和 `Close(ctx)`。

- `Init` 加载适配器配置，不启动监听。
- `Start` 恢复客户端绑定中已启用的 QQ 适配器，重复调用保留已有连接。启动时会尝试所有已启用适配器，返回汇总错误，失败原因也保留在适配器状态中。
- `Close` 取消所有适配器任务并释放连接，可重复调用。即使传入的上下文已取消也执行清理，然后返回取消错误；关闭会等待底层连接资源释放。

`Backend.Init()` 将 `&Platform.QQ` 注册为 `"qq"`，注册表和原有 `Core` QQ API 共用同一个实例：

```go
qq, err := backend.Platform.Servers.Get[*backend.QQServer](backend.QQServerName)
if err != nil {
    return err
}
if err := qq.Init(ctx); err != nil {
    return err
}
if err := qq.Start(ctx); err != nil {
    return err
}
```

`Init` 要在宿主已设置 `Paths` 和 `FS` 后调用。通过 `Start(ctx)` 新启动的适配器随该上下文取消而关闭；已有适配器的上下文不会被重复启动替换。原有 `Core.StartQQAdapter` 和 `Core.StartExtensionClients` 仍使用后台上下文，统一退出时调用 `Close`。

手动创建 `Core` 时，可用 `backend.NewQQServer(core)` 绑定其 QQ 服务。构造函数返回 `&core.QQ`，不复制状态。使用后不要整体复制 `QQServer`，内部包含互斥锁。
