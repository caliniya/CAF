# CAF

CAF 是 Go 编写的 Agent 框架工程骨架，界面使用 Wails v2.17.0。现阶段优先 Windows x64；模型、工具执行和 QQ 登录业务尚未实现。

仓库：`caliniya/CAF`；原 Java/Arc 项目：`caliniya/CAFOrigin`。Go module 为 `github.com/caliniya/caf`。

## Windows x64 开始使用

准备 Go 1.25+（本次使用 1.27.3 验证）和 WebView2 Runtime：

```powershell
git submodule update --init --recursive
.\dev.ps1       # 开发运行，默认使用 build/dev-data/
.\build.ps1     # 输出 build/bin/CAF.exe
.\test.ps1      # Go 测试、vet；有 Node 时也运行前端测试
```

脚本使用固定版本 Wails CLI，不要求全局安装。首次下载 Go 模块和子模块需要联网。若 PowerShell 禁止本地脚本，可直接运行：

```powershell
go mod download
go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -platform windows/amd64
go test -count=1 ./tests
```

前端是纯 HTML/CSS/JS，没有 npm 依赖或前端打包步骤。Node 仅用于可选的前端测试。应用不需要 JRE；已有 NativeAOT DLL 不要求另装 .NET 运行时，但 Windows 窗口仍需要 WebView2。

## 开发数据目录与 VS Code

开发数据放在 `build/dev-data/`，不再占用工程根目录的 `data/`；发布版默认路径不变。
开发脚本按 `-DataDir` 参数 → 已有 `CAF_DATA_DIR` 环境变量 → 默认路径的顺序选择目录。
脚本中的相对覆盖路径以项目根目录为基准，转换成绝对路径再交给 Wails，避免跟着开发 EXE 的位置变化：

```powershell
.\dev.ps1 -DataDir 'build/my-dev-data'
.\dev.ps1 -DataDir 'D:\CAF-data'
```

脚本退出（包括报错）后会原样恢复环境变量及当前目录。旧 `data/` 不会自动删除或迁移；
若要保留旧开发数据，请先关闭程序，再把内容迁到新目录。开发数据仍可能包含凭据，不能当作可随意清理的构建产物。

在 VS Code 中打开项目文件夹或 `CAF.code-workspace`，安装官方 Go 扩展（`golang.go`）：

- **F5**：在“运行和调试”选择 `CAF: Debug (Windows x64)`。预启动任务调用固定版本 Wails CLI，使用 `-debug` 构建 `build/bin/CAF-debug.exe`，然后由 Go/Delve 调试器启动；可打 Go 断点。第一次使用按扩展提示安装/更新 `dlv`。
- **Ctrl+Shift+P → Tasks: Run Task → CAF: Wails dev**：等同 `dev.ps1`，用于 Wails 自动重建/刷新；也可直接在终端执行脚本。它不是 F5 断点调试。
- 两种方式都默认使用 `build/dev-data/`。修改 F5 目录时，改 `.vscode/launch.json` 的 `env.CAF_DATA_DIR`，建议使用 `${workspaceFolder}` 开头的绝对路径。
- 不再使用原来的 `Launch Package`、`go run main.go` 或 Code Runner 的“运行当前文件”。这些入口不会自动执行 Wails 构建流程，缺少 Wails 的 build tags；也不会执行 dev.ps1 的目录设置。
- F5 使用内嵌页面，不提供 dev 模式的热更新；改代码后停止并重新 F5。普通浏览器预览也不等于具备 Go 绑定的 Wails 窗口。

`.vscode/launch.json`、`tasks.json`、`extensions.json` 是共享配置，纳入 Git；其他个人编辑器配置仍忽略。
调试 EXE 与发布的 `CAF.exe` 分开，开发数据目录也被 Git 忽略，不需要提交。
这组调试配置面向当前 Windows x64 开发环境，不修改核心平台接口。
配置方式参考 [Wails IDE 指南](https://wails.io/docs/guides/ides/) 和 [Go 扩展调试文档](https://github.com/golang/vscode-go/wiki/debugging)。

## 项目结构

```text
main.go                     嵌入前端、选择桌面后端、组装入口
core/runtime.go             统一持有路径与后端，管理生命周期、错误和调用
core/platform/              后端接口、路径数据、原生状态与公共错误
core/app/                   面向应用的服务与状态 DTO
core/agent/                 Agent 业务占位
platforms/desktop/backend.go 桌面后端，实现 core/platform.Backend
platforms/desktop/paths.go   桌面默认目录、相对路径解析
platforms/desktop/ui.go      Wails 绑定门面，调用核心服务
platforms/desktop/window/   Wails 窗口配置（与纯 Go 后端分开）
platforms/desktop/native/   Windows x64 goinvoke 装载、导出解析
platforms/desktop/natives/  内置 DLL、来源记录、提取与缓存
platforms/android/          仅占位，目前没有安卓构建或实现
frontend/                   HTML/CSS/JS 页面与调用桥接
tests/                      集中的 Go 黑盒测试、前端测试
lagrange/                   固定版本 Lagrange.Core Git 子模块
patches/                    NativeAPI 编译兼容补丁
scripts/build-native.ps1    可选的 Windows 原生库重编脚本
```

业务不再放在 `internal/`。`core` 不依赖桌面、Wails、goinvoke 或嵌入的 Windows DLL；`platforms/desktop` 依赖核心契约，不反向注册到核心，不使用全局运行时或 Go `init()` 偷偷初始化。

## 核心与平台如何接起来

`core/platform.Backend` 定义：

- `Init(Paths)`：使用确定的路径准备运行环境；不自动装载 DLL 或登录。
- `NativeStatus()`：无副作用的状态查询。
- `LoadNative()`：显式装载并解析导出，不创建 QQ 会话。
- `BeginLogin(context.Context)`：发起登录的契约；当前桌面返回 `ErrLoginNotImplemented`。
- `Close()`：结束该后端的生命周期；不卸载 NativeAOT DLL。

`core.NewRuntime(paths, backend)` 只校验并保存引用，`Runtime.Init()` 才调用后端初始化。路径以值拷贝保存，通过 `Runtime.Paths()` 读取；没有运行途中随意改路径的 setter。调用方使用 `Runtime.LoadNative()` / `Runtime.BeginLogin(ctx)`，无需知道平台或原生地址。

`core/app.Service` 使用同一个 Runtime；桌面 `App` 只是界面门面。入口选择 `desktop.New()` 并注入 `desktop.NewApp(backend)`，Wails 的 Startup 回调才解析目录、创建 Runtime 并执行 Init。构造对象或生成绑定不会创建目录、加载 DLL 或登录；Shutdown 调用 Close。

当前界面绑定为 `window.go.desktop.App`，只暴露 GetStatus、LoadNativeLibrary、BeginLogin。Startup / Shutdown 是生命周期回调，不作为 JS 业务接口。前端通过 `frontend/bridge.mjs` 集中调用，不到处散落平台判断。

**登录契约不等于登录已完成：**目前没有配置结构体封送、会话管理、二维码/在线事件处理。BeginLogin 明确返回未实现错误，不会伪装成功，也不会用猜测的参数调用 Initialize/Start。界面仍只有状态查看和本地库装载按钮。

## 便携目录与二进制

- 发布版默认将数据写入 EXE 旁的 `data/`；开发脚本使用 `build/dev-data/`。
- `CAF_DATA_DIR` 可覆盖数据目录；相对值始终相对 EXE 目录，而不是启动工作目录。平台解析为绝对路径后再传给核心。
- Windows DLL 源文件位于 `platforms/desktop/natives/win-x64/Lagrange.Core.NativeAPI.dll`，复用已有二进制，不含 PDB。
- DLL 内嵌 EXE，显式加载时提取到 `data/natives/win-x64/<SHA-256>/`。相同内容复用，升级使用新目录，避免覆盖被锁定的 DLL。
- NativeAOT 映射保留到进程退出，Close 不调用 Release/FreeLibrary。
- 目前只有 Windows x64 的原生实现；其他桌面平台返回明确的未支持错误。安卓仍需单独验证运行壳、数据目录和 Android/Bionic 原生库，不能复用 Windows DLL。

不打开窗口，只检查装载：

```powershell
go run ./cmd/nativecheck -data-dir "$PWD/data"
```

## 测试

所有自有 Go 测试均在 `tests/`，使用 `package tests` 和公开接口；仍是同一个 Go module。原生缓存测试检查提取结果，不访问或额外导出包内变量。

```powershell
go test -count=1 ./tests
go test -race -count=1 ./tests  # 可选；需要对应 C 编译工具链
go vet -tags bindings ./...
node --test tests/frontend.test.mjs
.\tests\development.test.ps1  # 无窗口的开发脚本与 VS Code 配置检查
```

`go test ./tests` 不依赖 Wails/GTK；整个工程的无窗口检查使用 `go test -tags bindings ./...`。Linux GUI 仍需 GTK/WebKit 开发库。测试分工和覆盖率命令见 [tests/README.md](tests/README.md)。

## 可选：重编 NativeAPI

改动 C# 或原生补丁时，在 Windows 上准备 .NET 10 SDK、Visual Studio C++ 桌面工具链和 Windows SDK：

```powershell
.\scripts\build-native.ps1
.\build.ps1
```

脚本将固定子模块源码导出到 `build/native-src/`，在副本应用 `patches/*.patch`，发布后更新桌面资源目录中的 DLL 和 PROVENANCE.json。不会修改子模块，也不会记录无法推送的本地子模块提交。

验证边界见 [VALIDATION.md](VALIDATION.md)，第三方来源见 [THIRD_PARTY.md](THIRD_PARTY.md)。
