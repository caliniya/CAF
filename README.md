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
  "signServer": "",
  "signToken": "",
  "protocol": "linux",
  "version": "3.2.19-39038"
}
```

1. 在本机文件中填完整的签名接口地址；需要认证时再填 signToken。远程服务优先使用 HTTPS。
2. protocol/version 必须和签名服务对应；这里是 QQ 协议版本，不是 CAF 宿主操作系统。
3. 回到界面点击“登录 QQ”，等待本机二维码出现，用手机扫码确认。
4. 界面必须显示“已上线”才能手动发送群文本；登录本身不会自动发送任何测试消息。
5. “取消 / 断开 QQ”结束当前会话；关闭程序也会取消任务并释放 SDK 客户端。

默认版本只是 SDK 已知版本，不保证签名服务支持它。SDK 的签名 Provider 是 HTTP POST JSON，
请求字段含 command/seq/body/uin/guid/qua，响应含 value.sec_sign/sec_token/sec_extra，认证使用 Bearer token。
不能把任意“签名服务器”地址直接当成兼容服务；示例不提供公共服务或令牌。

二维码、signToken、device.json 和 sig.bin 都只应保存在本机，不能发到群里、提交到 Git 或附在日志中。
核心状态不返回签名配置和凭据；原始 SDK 日志与报文转储暂不输出，界面只显示经过简化的错误。
因此发生 SDK 登录失败时，需要结合服务端的安全诊断进一步排查，而不是把完整认证报文贴出来。

登录设备标识保存在 qq/device.json，成功登录时的凭据保存在 qq/sig.bin。
下一次优先 FastLogin，失败才重新取二维码。损坏的设备/凭据文件会明确报错，不会偷偷重置或删除。
二维码文件 qq/qrcode.png 在登录完成、失败或取消后清除；Linux 新建敏感文件权限为 0600，Windows 仍需正确设置目录访问权限。

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
- `GetStatus().QQ`：查询 phase、二维码、上线账号与简化错误。
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
