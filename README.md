# CAF

CAF 是 Go 编写的 Agent 框架骨架，界面使用 Wails v2.17.0，现阶段优先 Windows x64。
模型、工具执行和真实 QQ 登录尚未实现。

仓库：`caliniya/CAF`；原 Java/Arc 项目：`caliniya/CAFOrigin`。
Go module：`github.com/caliniya/caf`。

## 开始使用

准备 Go 1.25+、Windows WebView2 Runtime；本次使用 Go 1.27.3 验证：

```powershell
git submodule update --init --recursive
.\dev.ps1       # 开发运行，数据默认放在 build/dev-data/
.\build.ps1     # 发布输出 build/bin/CAF.exe
.\test.ps1      # 核心、平台、脚本和前端测试
```

脚本调用固定版本 Wails CLI，无需全局安装。前端是纯 HTML/CSS/JS，没有 npm 构建依赖；
Node 仅用于可选前端测试。程序不需要 JRE；内置 NativeAOT DLL 不要求另装 .NET 运行时。

## 核心在哪里，怎么调用

**全局入口就是 `core.Platform`，类型为 `*core.Core`，定义在 `core/core.go`。**
它统一持有路径、状态、上下文和平台接口引用。没有额外的 desktop.App 或 Service。

```text
main 选择桌面实现 → 创建一个 Core → 赋给 core.Platform
                                  ↓
Wails 直接绑定同一个 *core.Core ← 其他 Go 包访问 core.Platform
                                  ↓
                       Backend / QQPlatform 接口
                                  ↓
                            桌面平台实现
```

`main.go` 是唯一的应用装配位置：

```go
// 由入口决定平台，核心不导入 desktop，也不自己猜测平台。
backend := desktop.New()
paths, err := desktop.DiscoverPaths(os.Getenv("CAF_DATA_DIR"))
if err != nil {
    log.Fatal(err)
}
core.Platform, err = core.NewCore(paths, backend)
if err != nil {
    log.Fatal(err)
}
// 窗口直接绑定这一个指针，不创建第二份核心。
if err := window.Run(assets, core.Platform); err != nil {
    log.Fatal(err)
}
```

应用启动完成后，其他 Go 包可直接调用：

```go
// 这些调用都作用于同一个核心，不需要重新 NewCore。
paths := core.Platform.Paths()
status := core.Platform.GetStatus()
status, err := core.Platform.LoadNativeLibrary()
status, err = core.Platform.BeginLogin()
```

以上为分别调用的示例；实际代码按需接收返回值并处理 error。
全局入口只在 main 启动阶段赋值，不能在运行时反复替换，也不要把 `*core.Core` 解引用后按值复制。
测试或命令行检查可用 NewCore 创建独立实例，但不会自动覆盖全局入口。

## 核心与平台的职责

- `core/core.go`：核心实例、全局入口、初始化、关闭、加载与登录请求编排。
- `core/status.go`：核心状态和页面数据，不另设转发服务。
- `core/platform.Backend`：平台基础能力，只定义 Init、NativeStatus、LoadNative、Close。
- `core/platform.QQPlatform`：独立的 QQ 能力，目前定义 BeginLogin(context.Context) error。
- `platforms/desktop`：目录解析、本地库装载等具体实现；同一桌面对象实现上面两个接口。
- `platforms/desktop/window`：只负责 Wails 窗口配置，把核心的 Startup/Shutdown 接到窗口生命周期并直接 Bind 核心。

核心通过接口调用后端，不 import desktop、Wails、goinvoke 或 Windows DLL 资源。
QQPlatform 是跨平台业务能力，不照抄裸 DLL 的 Start(index) 整数签名；
原生索引、结构体封送、事件内存与状态码仍由后端处理。基础后端可以不提供 QQ 能力，此时核心明确返回 ErrQQUnavailable。

## 生命周期与页面绑定

1. `NewCore(paths, backend)` 只校验与保存引用；构造时不创建目录、不加载库、不登录。
2. Wails 的 `Startup(ctx)` 直接通知核心；核心保存生命周期上下文并初始化一次后端。
3. 页面用 `GetStatus`、`LoadNativeLibrary`、`BeginLogin` 调用核心，不传 Go 的 context 对象。
4. `Shutdown(ctx)` 直接关闭同一核心，取消其子上下文，只关闭后端一次；NativeAOT 库不会卸载。

初始化失败会保留原始错误，允许重试。关闭后不能再次初始化或启动登录。
无界面程序也可使用 `Init()` / `LoadNative()` / `Close()`。

前端命名空间已改为 **`window.go.core.Core`**；`frontend/bridge.mjs` 统一处理它。
Startup/Shutdown 已接为 Wails 生命周期回调，不作为 JS 调用方法；其余公开、参数可序列化的核心方法会生成绑定。
平台接口和原生函数指针都保留在核心私有字段，不直接暴露给页面。

**BeginLogin 的链路已接通，不等于真实登录已实现。**
当前桌面实现明确返回“QQ 登录尚未实现”，不会猜测参数调用 Initialize/Start，也不会假报上线。
没有新增二维码解码、登录凭据持久化、消息收发或 Agent 执行功能。

## 目录与开发调试

- 发布版默认使用 EXE 旁的 `data/`；开发脚本和 F5 使用 `build/dev-data/`。
- dev.ps1 的优先级是 `-DataDir` 参数 → 已有 CAF_DATA_DIR → 默认目录。
- 开发脚本将相对覆盖路径按项目根目录转成绝对路径；程序自身接收相对 CAF_DATA_DIR 时仍按 EXE 目录解析。
- 旧 data/ 不自动删除或迁移。开发数据可能包含凭据，不能当作可随意删除的构建产物。

```powershell
.\dev.ps1 -DataDir 'build/my-data'
```

VS Code 打开项目目录或 CAF.code-workspace，安装官方 Go 扩展：

- F5 选择 `CAF: Debug (Windows x64)`：先由 Wails 构建 CAF-debug.exe，再由 Delve 启动；首次调试用 `Go: Install/Update Tools` 安装 dlv。
- 自动重建/刷新用终端执行 dev.ps1，或运行任务 `CAF: Wails dev`。它不是断点调试。
- 不用“运行当前文件”或普通 Launch Package 代替 Wails 构建。
- F5 的目录由 `.vscode/launch.json` 中 CAF_DATA_DIR 配置；修改源码后需停止并重新 F5。

## 原生库与后续平台

DLL 位于 `platforms/desktop/natives/win-x64/Lagrange.Core.NativeAPI.dll`，原样嵌入 EXE，不含 PDB。
显式加载时提取到数据目录的 `natives/win-x64/<SHA-256>/`，按内容复用缓存，不覆盖正在加载的 DLL。
映射保留到进程退出，不调用 Release/FreeLibrary。本次没有修改二进制或 Lagrange 子模块。

```powershell
go run ./cmd/nativecheck -data-dir "$PWD/build/dev-data"
```

Android 只保留占位目录；后续提供自己的平台接口实现与窗口壳，不复用 Windows DLL。
可选重编 NativeAPI 仍使用 `scripts/build-native.ps1`，在源码副本上应用补丁，需要 .NET 10 和 Windows C++ 工具链。

## 验证

```powershell
.\test.ps1
go test -race -count=1 ./tests
go test -tags bindings -count=1 ./...
```

全部自有测试集中在 tests/。检查分工见 [tests/README.md](tests/README.md)，
结果与尚未实测的边界见 [VALIDATION.md](VALIDATION.md)，应用本次补丁见 [MIGRATION.md](MIGRATION.md)。
