# 当前验证：Git 同步与 QQ 账号补丁修复

2026-10-21，远端已快进更新到 `537d964`。拉取前的本地 QQ 账号改动保存到 stash，没有覆盖未提交工作。
本地与远端没有分叉、冲突索引或未结束的 merge/rebase；Git 对象连通性和完整性检查均通过，未重写历史或强推。
原账号补丁检查稳定复现 QQSigner.go:11 上下文失配：远端比诊断版少了两行注释，账号改动尚未真正应用。
本轮按最新提交重新补齐账号改动并生成独立补丁；远端其他删除/新增均保留，不升级依赖、不修改上游 SDK。

- 新模板增加 qq: 0；旧配置缺省仍兼容，原有文件不覆盖。负数、越界、小数、字符串和布尔值在配置阶段拒绝。
- 非零 qq 只用于 SDK 签名请求的 uin 提示；客户端身份仍由真实扫码/缓存决定，不能据此假报上线。
- 本机模拟绑定校验的 HTTP 服务验证：扫码前补配置账号，扫码后保持相同实际账号，其他账号在发出签名请求前拒绝。
- 扫码账号不匹配时，在调用 QRCodeLogin 前终止；上线/保存前再次校验。其他账号的缓存在真实 SDK 联网前拒绝，旧 sig.bin 不删除或覆盖。
- test.ps1：62 个 Go 测试、bindings vet、7 次脚本模拟与 VS Code 检查、9 个前端测试通过。
- 全部 Go 测试 race、新增账号绑定测试连续 10 轮 race、全工程 bindings 测试、Windows x64 包构建与 production vet 通过。
- Wails Windows x64 production 交叉构建通过；生成模型不含 QQConfig 或签名配置，生成绑定不初始化数据目录。
- Linux 无 CGO 的 qqcheck 编译及模板/非法 qq/协议版本错误冒烟检查通过；未输出配置号码、令牌或地址。
- 模块校验、tidy -diff、gofmt、diff 空白检查通过。

仍未用真实签名服务鉴权、扫码或发消息；403 是否由 uin=0 引起、服务是否接受现有 JSON uin 字段，仍须用户本机复测。
没有将填号码等同于绕过鉴权或成功登录；Windows GUI 实机及 Linux GTK/WebKit 窗口运行未验证。

---

# 上一轮验证：QQ 失败诊断

2026-10-21，已将 CAF 快进更新到 `01587d4`。拉取前的本地改动保留在 stash，没有强制覆盖或重复应用已合并的旧补丁。

确认的问题：“登录或连接失败”是前端统一状态名；底层原因被通用提示替代，且 SDK Provider 不会拒绝 HTTP 200 中的非零业务 code 或缺失签名字段。
已增加阶段/错误码/中文原因，并在后端包装签名响应校验，不修改上游 SDK 源码。

## 本次通过

- test.ps1：55 个 Go 测试、bindings vet、7 次开发脚本模拟与 VS Code 配置检查、9 个前端测试。
- 全部集中测试的 race 检查，以及新增签名/诊断测试连续 10 轮 race 检查。
- go test -tags bindings ./...、Windows x64 无窗口包构建与 production vet。
- Wails Windows x64 production 构建；生成的 QQStatus 模型包含 stage/errorCode，不包含签名配置；绑定生成不创建测试指定的数据目录。
- Linux 无 CGO 的 qqcheck 编译；缺配置和不支持的协议版本均输出对应阶段/错误码，未创建设备或凭据。
- 本机 httptest 验证真实 SDK Provider 的 POST 字段、Bearer 认证与 User-Agent；只使用虚构数据，未连接外部签名服务。
- HTTP 401/403/404/405/429/502、非零 code、HTML/错误十六进制、旧字段格式/空签名均被拒绝，响应正文、令牌和 URL 不进入页面状态。
- Go 模块校验、tidy -diff、gofmt、前端语法与 diff 空白检查。

## 仍未验证

没有用真实签名服务和账号扫码，也没有向真实群发消息；仅凭统一状态名仍无法确定用户本机失败的根因。
需应用补丁重试并查看具体阶段和错误码，不宣称真实登录已修复。Windows 窗口未在实机运行；Linux GUI 仍受当前宿主缺少 pkg-config/GTK/WebKit 限制。

---

# 历史：LagrangeGo 试验后端验证

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
