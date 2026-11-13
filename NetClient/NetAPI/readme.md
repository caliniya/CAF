# 网络 API 客户端

`TimeClient` 使用 `https://caf-netapi-caliniya.pages.dev/api/time` 获取时间，结果统一为北京时间（UTC+08:00），保留服务器原始 UTC 时间和请求标识。

客户端应长期复用同一个实例：首次调用 `Now(ctx)` 请求服务器，之后使用服务器时间基准加单调时钟经过的时间计算当前值。默认每 5 分钟在下一次调用时校准，并发校准共享一个请求。没有调用时不会发送请求，通常连续运行 24 小时约使用 288 次请求，显示刷新频率不影响此数量。

```go
client := netapi.NewTimeClient("")
client.SyncInterval = 5 * time.Minute
client.RetryInterval = 30 * time.Second

sample := client.Now(ctx)
// sample.Time：当前估计时间。
// sample.Cached、sample.SampleAgeMS：是否推算自缓存、基准距今多久。
// sample.ServerUTC、sample.RequestID：最近成功取时的原始证据。
// sample.Source：cloudflare 或 local。
// sample.FallbackReason：本机回退原因。

// 网络恢复或需要立即校准时，可主动请求一次。
sample = client.Sync(ctx)
```

取时失败时保存本机时间基准并明确标记 `local`，默认 30 秒后才在下一次调用时重试。请求最长等待 10 秒（`Timeout` 可配置）。实例开始使用后不要复制它，也不要并发修改配置。

HTTP 连接可由 Go 自动复用，但每次 HTTP 请求仍需要服务器执行。节省请求来自复用取时基准，不需要服务器保持长连接或改用 WebSocket，现有 Pages 网站无需重新部署。

网络往返耗时的一半只是延迟补偿估计；基准越久未校准，晶振误差可能越大。`Cached` 时间是本地推算值，`SampleAgeMS` 不是误差上限。单调计时不受系统时间前后调整影响，但部分系统的睡眠会暂停单调计时；设备唤醒后应主动 `Sync`。版本顺序仍由业务层用序号或版本关系保证。
