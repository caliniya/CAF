# 记忆取时

默认时间接口：`https://caf-netapi-caliniya.pages.dev/api/time`。

`MemoryDate()` 返回当前 UTC 的 RFC3339Nano 字符串。每次调用优先在线取时，最多等待 4 秒，失败时使用本机 UTC。

需要追溯来源时，使用 `MemoryTimestamp(ctx)` 并保留整个结果：

```go
stamp := Memory.MemoryTimestamp(ctx)
// stamp.UTC：加上网络往返耗时一半后，估计的接收时刻。
// stamp.ServerUTC：Cloudflare 服务器原始时间，本机回退时为空。
// stamp.Source：cloudflare 或 local。
// stamp.RequestID、stamp.Endpoint：服务器请求标识与接口地址。
// stamp.RoundTripMS：成功取时的网络往返耗时，单位毫秒。
// stamp.FallbackReason：失败原因，正常取时为空。
```

`NetClient.NewTimeClient(endpoint)` 可创建自定义地址或超时的客户端；传空地址使用默认接口。它不依赖模型 API 密钥，也不修改系统时钟。底层 HTTP 使用正常 TLS 证书验证；每次请求携带随机 nonce，并校验响应 nonce、时间字段一致性和协议版本。

网络补偿是假设往返延迟对称的估计，不能证明绝对误差；服务器时间也不是签名时间戳。持久化顺序仍应使用版本关系或数据库序号。这里仅提供取时能力，尚未实现记忆的持久化。
