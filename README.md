# CAF

CAF 是 Go/Wails 桌面 Agent 框架骨架。QQ 桌面后端已换成 LagrangeGo 的最小试验实现，
支持显式扫码登录、尝试复用凭据、取消会话和手动发送一条群文本。
模型、工具执行、OneBot 接入与消息事件转发尚未实现。

当前没有做真实 QQ 登录验证；签名服务、协议版本与账号状态仍需要在本机实测。

## 依赖

- Go 1.26+，本次使用 Go 1.27.3。
- Wails v2.17.0；Windows 窗口需要 WebView2 Runtime。
- LagrangeGo 固定版本 `v0.1.5-0.20260718062712-f7f8abfaa7f1`，通过 Go modules 引入。

已移除 Lagrange.Core Git 子模块、goinvoke、旧 FFI 绑定和 C# 重编入口。
不再要求 .NET、NativeAOT 或 Lagrange DLL/so；Wails 自己的图形系统依赖另算。
SDK 带来的间接依赖更新记录在 go.mod/go.sum，不跟随 master 自动漂移。

```powershell
.\dev.ps1       # 本地界面；数据默认位于 build/dev-data/
.\test.ps1
.\build.ps1     # Windows x64 发布到 build/bin/CAF.exe
```

前端是纯 HTML/CSS/JS，没有 npm 构建步骤；Node 只用于可选测试。
不再执行 git submodule update，也不再运行 scripts/build-native.ps1。

## 第一次试验：只在本地配置

首次初始化会在数据目录创建 `qq/config.json` 空白模板，不联网、不自动登录：

```json
{
  "qq": 0,
  "signServer": "",
  "signToken": "",
  "protocol": "linux",
  "version": "3.2.19-39038"
}
```

1. 在本机文件中填完整的签名接口地址；需要认证时再填 signToken。远程服务优先使用 HTTPS。若服务绑定了 QQ，在 qq 中填绑定号码（数字，不加引号）。
2. protocol/version 必须和签名服务对应；这里是 QQ 协议版本，不是 CAF 宿主操作系统。
3. 回到界面点击“登录 QQ”，等待本机二维码出现，用手机扫码确认。
4. 界面必须显示“已上线”才能手动发送群文本；登录本身不会自动发送任何测试消息。
5. “取消 / 断开 QQ”结束当前会话；关闭程序也会取消任务并释放 SDK 客户端。

默认版本只是 SDK 已知版本，不保证签名服务支持它。SDK 的签名 Provider 是 HTTP POST JSON，
请求字段含 command/seq/body/uin/guid/qua，响应含 value.sec_sign/sec_token/sec_extra，认证使用 Bearer token。
不能把任意“签名服务器”地址直接当成兼容服务；示例不提供公共服务或令牌。

`qq` 默认为 0，旧配置没有此项时仍保持自动识别行为。已有 config.json 不会被覆盖，需手动增加这一项。
例如把 `"qq": 123456789,` 加进现有 JSON 对象，并将示例数字替换为签名服务绑定的账号；保留原有接口、令牌和协议版本。
保存后重新发起登录；如果会话尚在运行，先取消/断开。

扫码确认前，SDK 的账号为 0 时，签名 Provider 会把配置 qq 作为签名请求的 `uin`；不会强改 QQ 客户端的登录身份或报文。
确认扫码账号后必须与配置一致，否则停止登录，不保存凭据；缓存属于其他账号也会在联网前拒绝，不自动删除文件。
这里仅适配 SDK 已有 JSON `uin` 字段；如果服务另要求查询参数或请求头，仍需按服务文档适配。
填号码不能替代有效令牌、绑定授权和协议兼容性，也不保证单凭此项即可消除 403。

二维码、signToken、device.json 和 sig.bin 都只应保存在本机，不能发到群里、提交到 Git 或附在日志中。
核心状态不返回签名配置和凭据；原始 SDK 日志与报文转储暂不输出，界面显示经过白名单处理的失败阶段、错误码和中文原因。
未分类的底层异常仍不直接输出；需要时结合服务端的安全诊断排查，不要贴完整认证报文。

登录设备标识保存在 qq/device.json，成功登录时的凭据保存在 qq/sig.bin。
下一次优先 FastLogin，失败才重新取二维码。损坏的设备/凭据文件会明确报错，不会偷偷重置或删除。
二维码文件 qq/qrcode.png 在登录完成、失败或取消后清除；Linux 新建敏感文件权限为 0600，Windows 仍需正确设置目录访问权限。

### 显示“登录或连接失败”时

这只是通用状态名，不是根因。查看同一状态面板的“失败阶段 / 错误码 / 具体原因”；命令行 qqcheck 也会输出相同的脱敏诊断。

| 错误码 | 先检查什么 |
| --- | --- |
| `QQ_CONFIG` | 按界面显示的数据目录修改 qq/config.json，不要改错发布版/开发版目录 |
| `QQ_VERSION` | protocol/version 是否为当前 SDK 支持的成对值 |
| `SIGN_HTTP_401` / `SIGN_HTTP_403` | 签名服务认证和权限；在本机核对令牌、qq 绑定及接口规则 |
| `QQ_ACCOUNT_MISMATCH` | 实际扫码/登录账号与配置 qq 不一致，必须使用同一绑定账号 |
| `QQ_CACHE_ACCOUNT_MISMATCH` | 旧 sig.bin 属于其他账号；在本机备份并移走后重新扫码，程序不会自动删除 |
| `SIGN_HTTP_404` / `SIGN_HTTP_405` | 完整 POST 接口路径，不是服务首页 |
| `SIGN_REJECTED_<code>` | 服务已返回非零 code；核对版本、授权和服务接口 |
| `SIGN_RESPONSE_FORMAT` / `SIGN_EMPTY_SIGNATURE` | 签名协议是否兼容，响应需要 value.sec_sign；HTTP 200 不等于签名成功 |
| `SIGN_NETWORK_*` | 签名服务的网络连接、DNS 或超时 |
| `QQ_PACKET_TIMEOUT` | 等待 QQ 服务器回包超时，不能直接归因于签名服务 |
| `QQ_QR_REJECTED_<code>` / `QQ_LOGIN_REJECTED` | QQ 服务端拒绝；继续核对协议版本、签名兼容性和账号验证状态 |

可以提供阶段名和错误码帮助排查，不要附上配置、二维码、sig.bin 或认证报文。
签名检查只包装 SDK Provider，不实现签名算法、不自动试接口、不自动跳过签名，也不宣称修复了真实账号的登录。

### WSL 无窗口试验

不需要 GTK/WebKit 或 Lagrange .so，先在项目根目录运行：

```bash
go run ./cmd/qqcheck -data-dir ./build/dev-data
```

如果还没配置，会创建模板并提示配置后退出；配置好再运行。
需要扫码时，程序只打印本机 qrcode.png 的路径，不打印二维码内容、URL 或认证令牌。
上线后按 Ctrl+C 结束。只有同时提供 -group 和 -text 才会显式发送一条消息：

```bash
# 123456 仅为示例，替换为你有权限发送的测试群号。
go run ./cmd/qqcheck -data-dir ./build/dev-data -group 123456 -text "测试消息"
```

不会自动重复发送，返回成功仅表示 SDK 调用成功，不代表接收方已读。
SDK 方法不直接接受 context，取消通过中断 TCP 实现；签名 HTTP 和部分网络等待仍受 SDK 超时控制，不保证瞬间结束。

Wails 的实际 Linux 窗口仍需要 pkg-config、GTK/WebKit 及 WSLg 或其他显示环境。
准备好后可运行 `CAF_DATA_DIR="$PWD/build/dev-data" go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 dev`。
build.ps1 与 VS Code F5 仍面向 Windows x64。

## 结构与调用

```text
main.go                           创建后端，显式注入基础能力与 QQ 实现
core/                             全局 core.Platform、生命周期与业务编排
  platform/                       现有平台契约及普通状态类型
client/、pre/                     通信与解析占位
backend/desktop/
  DesktopBackend.go               目录初始化和关闭
  DesktopQQ.go                    后台登录任务与 QQ 状态
  QQConfig.go                     本地配置模板及私有文件写入
  QQSession.go                    后端内的可测试会话边界
  LagrangeSession.go               调用真实 LagrangeGo API
  QQSigner.go / QQDiagnostic.go    签名响应检查和脱敏错误分类
  window/                         Wails 窗口壳，直接绑定 core.Core
  natives/                        旧 DLL 与来源记录，仅归档，不加载或打包
backend/android/                  占位
frontend/                         本地登录状态、二维码和手动群文本界面
cmd/qqcheck/                      无窗口登录试验入口
cmd/nativecheck/                  旧命令兼容入口，只报告不再需要本地库
```

全局入口仍是 `core.Platform`，没有恢复 desktop.App 或第二个业务对象：

```go
backend := desktop.New()
core.Platform, err = core.NewCore(paths, backend, backend.QQ())
// 窗口仍绑定同一个指针。
```

- `BeginLogin()`：受理后台登录任务，不代表已经上线。
- `GetStatus().QQ`：查询 phase、二维码、上线账号、脱敏错误，以及失败时的 stage/errorCode。
- `CancelLogin()`：取消登录或断开当前会话，不关闭整个核心。
- `SendGroupText(group, text)`：明确请求发送一条群文本，要求已上线。
- 旧 LoadNative/LoadNativeLibrary 暂留兼容，始终返回 ErrNativeRetired，不能用它启动 QQ。

core/platform 不依赖 LagrangeGo；SDK 类型、签名配置和认证数据只留在后端。
设备与会话保存在数据目录，发布版默认是程序旁 data/，开发版仍是 build/dev-data/；已有数据不迁移、不清理。

## 归档与许可

旧 DLL 在 backend/desktop/natives/win-x64 原样保留，来源记录和历史补丁仍在，但不再进入发布程序的依赖图。
LagrangeGo 使用 AGPL-3.0，许可文本已保存到 LICENSES/。这不是 MIT 依赖，发布或分发前应核对授权要求。
具体来源见 THIRD_PARTY.md；本次没有替 CAF 选择自身许可证。

测试约定见 tests/README.md，验证结果与限制见 VALIDATION.md，补丁应用见 MIGRATION.md。
