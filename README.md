# CAF · Go

CAF 是一个 Agent 框架。本仓库先准备 **Go 核心、Wails 桌面入口和 goinvoke 本地库加载**，还没有实现模型对话、工具执行或 QQ 登录。

这是独立的 Go 工程，不修改旁边的 Java/Arc 版 CAF，也没有配置或推送 GitHub 远程仓库。Go module 名称为 `github.com/caliniya/CAF-Go`，应用名仍为 CAF。

## Windows x64 开始使用

准备 Go **1.25+**（本次使用 1.27.3 验证）和 Windows WebView2 Runtime。

```powershell
git submodule update --init --recursive
.\dev.ps1       # 开发运行
.\build.ps1     # 发布构建：build/bin/CAF.exe
.\test.ps1      # Go 测试/vet；安装了 Node 时也运行前端测试
```

脚本使用固定版本 Wails CLI，不要求全局安装 `wails`。首次需要联网下载 Go 模块；PowerShell 若禁止本地脚本，可直接运行对应命令：

```powershell
go mod download
go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -platform windows/amd64
go test -tags bindings ./...
```

这个最小前端是纯 HTML/CSS/JavaScript，直接嵌入应用；没有 Vite、Vue/React、npm 依赖或前端构建步骤。Node 仅供可选的前端单元测试使用。以后需要前端框架再增加构建链。

## 结构与依赖

```text
main.go / app.go       Wails 入口及很薄的界面绑定
frontend/              原生 HTML/CSS/JS 页面
internal/app/          不依赖 Wails/goinvoke 的应用服务与接口
internal/paths/        相对可执行文件组织可写数据目录
internal/lagrange/     Windows x64 goinvoke 加载、导出解析；其他平台明确报未实现
internal/agent/        Agent 业务占位，目前没有实现
natives/               已有 DLL、来源记录、嵌入与缓存释放
lagrange/              Lagrange.Core 固定版本 Git 子模块
patches/               NativeAPI 编译兼容补丁
scripts/build-native.ps1 可选的 Windows 原生库重编脚本
platforms/android/     仅占位说明，没有 APK 或安卓实现
```

直接 Go 依赖只有 **Wails v2.17.0、goinvoke v1.3.3**；它们自己的间接依赖由 `go.mod/go.sum` 锁定。配置、HTTP/JSON、日志等后续优先使用标准库。

## 本地库与便携目录

- 复用 Java 版 CAF 已有的 Windows x64 `Lagrange.Core.NativeAPI.dll`，**不需要为了这个骨架重新编译它**。不包含 PDB，也不使用 JNA/Arc。
- DLL 内置 EXE，加载前释放到 `data/natives/win-x64/<SHA-256>/`。相同内容复用缓存，升级使用不同目录，避免覆盖锁定中的 DLL。
- 发布版数据默认在 EXE 旁的 `data/`；`dev.ps1` 则使用项目根目录的 `data/`，不放进构建输出。也可设置 `CAF_DATA_DIR`；相对值按 EXE 目录解析，不按启动工作目录解析。
- UI 按钮只加载库并解析 **6 个基础导出**，不调用 Initialize/Start，不扫码、不联网登录。它不是完整 QQ SDK；结构体封送、输出深层释放、会话管理和原生已知问题以后单独处理。
- 当前只实现 Windows x64 的实际加载；其他桌面平台没有配套原生二进制/加载实现，不能认为已经支持。Android 更不是 Wails 直接打包即可支持。
- NativeAOT 库不允许 `Release/FreeLibrary` 卸载；本实现将映射保留至进程退出。

只检查原生装载、不打开 UI：

```powershell
go run ./cmd/nativecheck -data-dir "$PWD/data"
```

Windows 仍需 WebView2。Go 不需要 JRE，NativeAOT DLL 不要求另装 .NET 运行时，但这不表示没有平台依赖。

## 以后重编 NativeAPI

只有改动 C# 逻辑/原生补丁时才需要 **.NET 10 SDK、Visual Studio C++ 桌面工具链和 Windows SDK**：

```powershell
.\scripts\build-native.ps1
.\build.ps1
```

脚本从子模块固定提交导出到 `build/native-src/`，在副本应用 `patches/*.patch`，发布 NativeAPI 并更新 DLL/校验记录。**不修改子模块、不引用无法推送的本地子模块提交**。副本和发布目录是可再生构建文件；不要在里面保存手工修改。

## 当前验证边界

此工程以 Windows x64 为主。Linux 环境可用 `go test -tags bindings ./...` 做无窗口测试；`bindings` 使用 Wails 的绑定生成后端，不需要 GTK/WebKit 窗口环境。直接构建 Linux GUI 仍需对应系统开发库。

具体已运行的检查及未验证项目见 [验证记录](VALIDATION.md)。仅构建通过不等于已经验证 Windows 窗口、QQ 登录或消息收发。

来源及第三方许可注意事项见 [THIRD_PARTY.md](THIRD_PARTY.md)。
