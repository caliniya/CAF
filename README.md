# CAF

CAF 是 Go 编写的 Agent 框架骨架，桌面界面使用 Wails v2.17.0。
当前先整理结构，模型请求、工具执行和真实消息收发尚未实现。

仓库：`caliniya/CAF`；原 Java/Arc 项目：`caliniya/CAFOrigin`。
Go module：`github.com/caliniya/caf`，所有自有包仍属于同一个 module。

## 目录与职责

```text
main.go                         装配后端、创建核心、启动窗口
core/                           核心状态、生命周期和业务总调度
  agent/                        Agent 业务占位
client/                         HTTP/WS 通信占位
platform/                       平台接口、路径和状态类型
backend/                        具体平台实现，不放核心业务
  desktop/
    DesktopBackend.go           桌面基础后端、路径初始化和关闭
    DesktopQQ.go                单独的 QQ 占位实现
    window/                     Wails 窗口壳，直接绑定核心
    native/                     保留的加载器、goinvoke 绑定及内存布局
    natives/                    内嵌 DLL、来源记录及提取缓存
  android/                      Android 占位
pre/                            协议解析和格式转换占位
frontend/                       HTML/CSS/JS 页面
cmd/nativecheck/                无窗口本地库状态检查
lagrange/                       Lagrange.Core Git 子模块
scripts/build-native.ps1         可选 Windows NativeAOT 重编脚本
tests/                          全部自有 Go、前端和脚本测试
```

- `client` 以后只处理网络收发、连接、超时和重连；目前不会监听端口或连接任何 API。
- `pre` 以后解析模型响应、工具调用描述和 OneBot 数据；目前没有协议解析实现。
- `core` 决定流程、查找已注册工具并校验参数和权限；不会把任意文本直接作为方法执行。
- `platform` 只定义契约和普通数据，不导入 `core` 或具体后端。
- `backend` 实现平台契约；原生库属于桌面后端细节，不再单设顶层 native 模块。

以后数据流是：网络数据 → pre 解析 → core 决策和执行 → client 发送结果。
这只是职责边界，本次没有提前实现 Agent 循环、工具执行或 OneBot 接入。

## 启动和平台注入

**全局核心入口仍然是 `core.Platform`，类型为 `*core.Core`。**
main 显式提供基础后端和 QQ 实现，核心不猜测后端是否兼任 QQ：

```go
backend := desktop.New()
paths, err := desktop.DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
if err != nil {
    log.Fatal(err)
}
// QQ() 只取得同一个 DesktopQQ 引用，不加载库、不登录。
core.Platform, err = core.NewCore(paths, backend, backend.QQ())
if err != nil {
    log.Fatal(err)
}
// 窗口绑定同一个核心，不另建 desktop.App 或 Service。
if err := window.Run(assets, core.Platform); err != nil {
    log.Fatal(err)
}
```

示例中的 desktop、window 分别来自 `backend/desktop` 和 `backend/desktop/window`。
接口改从 `github.com/caliniya/caf/platform` 导入；原来的 `core/platform`、`platforms/` 目录已迁移。

- `NewCore(paths, backend, qq)` 只保存引用；传 nil QQ 可以正常启动，登录时明确返回 ErrQQUnavailable。
- 带类型 nil 的 QQ 也按未提供处理，避免调用时崩溃；基础后端不能为 nil。
- Wails Startup 初始化同一个核心并保存生命周期上下文；Shutdown 取消上下文并关闭后端一次。
- 路径和平台引用由核心保存；应用启动后通过 `core.Platform.Paths()`、`GetStatus()`、`BeginLogin()` 调用。
- 全局指针只在 main 装配时赋值，不要复制带互斥锁的 Core，也不要在运行时反复替换全局引用。
- 页面仍使用 `window.go.core.Core`，不直接访问平台接口或原生函数指针。

测试和无窗口程序可以创建独立 Core，不会自动覆盖全局入口。

## QQ 暂停，不影响 WSL 测试

`platform.QQPlatform` 只保留 `BeginLogin(context.Context) error`。
桌面端由 `DesktopQQ.go` 中的 DesktopQQ 实现，与基础后端共用初始化和关闭状态。

当前加载和登录请求都返回 ErrQQIntegrationPaused；页面显示暂停并禁用加载按钮。
暂停不是启动失败，核心仍正常 ready；不会创建 QQ 会话、假报上线或加载本地库。

Windows DLL 移到 `backend/desktop/natives/win-x64/Lagrange.Core.NativeAPI.dll`。
二进制内容、来源记录和 Lagrange 子模块提交不变，继续作为资源保留，不含 PDB。
正常启动和显式平台请求都不提取或装载它；旧数据与缓存不删除。
底层加载器和绑定仍保留，但不在当前应用流程中使用。WSL/Linux 测试无需准备 Lagrange `.so`。

```bash
go run ./cmd/nativecheck -data-dir "$PWD/build/dev-data"
```

检查程序当前只输出 status=paused，不创建指定的数据目录。
Windows 实际 DLL 装载测试默认跳过，仅在显式设置 CAF_TEST_LAGRANGE_NATIVE=1 时运行诊断。

## 构建和开发

准备 Go 1.25+；Windows 窗口需要 WebView2 Runtime。
脚本固定使用 Wails CLI v2.17.0；纯 HTML/CSS/JS 前端没有 npm 构建依赖。
Node 仅用于可选前端测试；程序不需要 JRE，也不因保留的 NativeAOT DLL 而要求另装 .NET 运行时。

```powershell
git submodule update --init --recursive
.\dev.ps1       # 数据默认在 build/dev-data/
.\build.ps1     # Windows x64 发布输出 build/bin/CAF.exe
.\test.ps1
```

VS Code F5 继续使用 Windows x64 的 Wails debug 构建和 Delve，不用“运行当前 Go 文件”代替。
需要首次安装调试器时使用官方 Go 扩展的 Go: Install/Update Tools 安装 dlv。

发布版数据默认位于程序旁的 data/；开发脚本优先级为 -DataDir → CAF_DATA_DIR → build/dev-data。
脚本把相对覆盖路径按项目目录转为绝对路径；程序直接接收相对 CAF_DATA_DIR 时按可执行文件目录解析。
开发目录可能包含凭据，不会因这次重构被删除或迁移。

### WSL/Linux

无窗口检查无需 GTK、WebKit 或 Lagrange `.so`：

```bash
go test -count=1 ./tests
go test -tags bindings -count=1 ./...
```

实际窗口仍需 Wails 的 Linux 编译依赖（含 pkg-config、GTK/WebKit）及 WSLg 或其他显示环境。
准备好后在项目根目录运行：

```bash
CAF_DATA_DIR="$PWD/build/dev-data" go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 dev
```

bindings 标签仅用于无窗口检查，不是运行窗口的方法。
build.ps1 与现有 F5 配置仍针对 Windows x64；Android 仍只占位。
可选 NativeAPI 重编脚本已同步新资源路径，但本次不重编或恢复 QQ。

## 验证与应用

```bash
go test -race -count=1 ./tests
node --test tests/frontend.test.mjs
```

测试说明见 [tests/README.md](tests/README.md)，真实验证结果与限制见 [VALIDATION.md](VALIDATION.md)。
结构迁移和补丁应用步骤见 [MIGRATION.md](MIGRATION.md)。
