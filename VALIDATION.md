# LagrangeGo 试验后端验证

2026-10-21，基于 CAF dd4f542，Go 1.27.3 / Wails v2.17.0 / PowerShell 7.6.7 / Node v18.20.4，Linux x64 宿主。
固定 SDK 版本为 v0.1.5-0.20260718062712-f7f8abfaa7f1。

## 已完成与通过

- test.ps1：50 个 Go 测试、整个工程 bindings 标签 vet、7 次开发脚本模拟和 VS Code 配置检查、8 个前端测试。
- 全部集中 Go 测试的 race 检查，以及 SDK 异步登录/取消测试连续 10 轮 race 检查。
- go test -tags bindings -count=1 ./...、Windows x64 的包构建、production vet 与测试程序交叉编译。
- Wails Windows x64 production 和 debug 构建均通过，页面绑定仍为同一个 core.Core。
- 实际生成 BeginLogin、CancelLogin、SendGroupText 和 GetStatus 的 JS；模型包含 QQStatus，但不包含签名配置、令牌或 SDK 会话对象。
- 绑定生成过程没有创建指定的全新 CAF_DATA_DIR，不初始化 QQ、不联网登录。
- production/debug EXE 中均不再包含原 C# DLL 资源；归档 DLL 本身与来源记录未改动。
- Linux 命令行 qqcheck 在 CGO_ENABLED=0 下编译通过；无签名配置时只创建空白模板并明确退出，没有创建 device.json 或 sig.bin。
- nativecheck 兼容入口只报告 not_required，不创建目录或加载任何库。
- 核心依赖图不包含 LagrangeGo、后端、Wails 或 goinvoke；Go 模块图已无 goinvoke。
- Go 模块校验、tidy -diff、gofmt、前端语法和 diff 空白检查通过。
- 在 dd4f542 的全新 Git 副本上按文档 deinit 并使用 --index 应用补丁，确认 gitlink/登记已删除；重新运行 test.ps1、全工程无窗口测试、race 和 Windows 包构建均通过。

## 测试覆盖的行为

- 构造和初始化不创建真实登录会话；缺少签名配置不影响核心 ready。
- 假会话验证：扫码确认、SDK 上线确认、凭据保存、缓存登录跳过二维码、重复登录保护、取消及关闭释放。
- 未上线不能发送群消息；登录后不自动发送，只有显式调用才发送一次，不自动重试。
- 假 SDK 的 nil 错误不等于已上线；未确认 online 时不保存凭据或假报成功。
- 签名配置和底层错误中的测试敏感标记不会进入页面状态；二维码结束后清除。
- 真实 SDK 仅验证离线对象构造、稳定设备标识、Linux 文件权限、非法版本和损坏设备文件不被覆盖。

成功状态来自假会话，只证明 CAF 编排可执行，不代表任何真实账号已经登录。

## 未验证与限制

- 没有配置真实签名服务、扫码登录或向真实群发送消息；没有取得或使用用户的认证令牌。
- 签名接口格式、协议版本、账号风控和当前 QQ 服务端兼容性必须由本机后续验证。
- SDK 方法本身不带 context，取消通过断开 TCP 实现，签名 HTTP/部分网络等待仍可能等到 SDK 超时。
- Windows GUI、Delve 断点未在 Windows 实机运行；本次是交叉构建。
- Linux production 图形编译仍因缺少 pkg-config 停止，GTK/WebKit/WSLg 窗口未验证；不影响无窗口 qqcheck。
- 不包含消息事件转发、OneBot、Agent 调度、Android 或生产级多账号管理。

Lagrange.Core 子模块登记和 gitlink 已移除；Git 对象缓存与旧 DLL/来源/历史补丁保留，不强制清理历史或用户数据。
