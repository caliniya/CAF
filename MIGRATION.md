# 本次迁移：LagrangeGo 桌面 QQ 试验后端

基于 `dd4f542`，补丁名 `CAF-lagrangego.patch`。包含依赖、源码、测试和许可，不包含任何账号配置。
需要 Go 1.26+。

本次会删除 Git 子模块登记和 gitlink。请在没有未保存改动的项目根目录执行，
先确保 lagrange 子模块自身没有改动，再解除工作目录初始化：

```powershell
git -C lagrange status --short
git submodule deinit lagrange
git apply --check --index .\CAF-lagrangego.patch
git apply --index .\CAF-lagrangego.patch
.\test.ps1
```

这里使用 --index，使子模块 gitlink 的删除也正确写入 Git 索引；源码改动也会一并暂存，但不会提交或推送。
如果从未初始化过 lagrange，可以跳过 deinit；未初始化目录的清理警告不影响后续补丁检查。
如果 --check 报错，不要强行覆盖，也不要运行 git clean 或 reset --hard；先保存现有工作。
旧子模块的 Git 对象缓存和历史提交没有强制删除，便于回溯。

主要变化：

- LagrangeGo 通过 Go modules 固定到 f7f8abfaa7f1；升级 Go 声明到 1.26，间接依赖按模块选择规则更新。
- 删除 Lagrange.Core 子模块、旧 native FFI 源码、goinvoke 与 NativeAOT 构建入口。
- 原 DLL、来源记录和历史补丁原样保留归档，但应用不再引用 natives 包，不会把 DLL 打进 EXE。
- DesktopQQ 换成实际 Go SDK 适配；登录在后台执行，不阻塞状态查询。
- QQPlatform 的 BeginLogin 保留；状态、取消和群文本是独立的可选接口，不破坏最小假平台测试。
- 页面改为本机扫码登录和手动发送群文本，增加 cmd/qqcheck 供 WSL 无窗口试验。
- 没有恢复 OneBot、消息事件转发或 Agent 业务；没有替用户填写签名服务或自动发测试消息。

首次运行后在数据目录 qq/config.json 本地配置签名服务。具体步骤见 README，不要将令牌或二维码发到群里。
旧数据不删除、不迁移；新的敏感文件加入 Git 忽略规则。
