# 本次验证记录

2026-10-20，基于 830e209；Linux x64 环境，Go 1.27.3、Wails CLI v2.17.0、PowerShell 7.6.7、Node v18.20.4。

## 已通过

- test.ps1：36 个 Go 测试、bindings 标签的整个工程 vet、7 次开发脚本模拟及配置检查、5 个前端测试通过。
- go test -race -count=1 ./tests：通过。
- go test -tags bindings -count=1 ./...：整个工程的无窗口编译与测试通过。
- Windows x64 production/debug vet 通过；整个测试程序和 nativecheck 交叉编译通过。
- go mod verify 和 go mod tidy -diff 通过，没有新增或修改依赖。
- gofmt、前端 node --check 与 git diff --check 通过。
- core 的依赖检查通过：不依赖具体平台、Wails、goinvoke 或 DLL 资源包。
- 目标包覆盖率 87.4%，仅指本次 Linux 参与构建的 core、桌面后端和原生缓存代码，不代表 Windows 装载或登录覆盖率。
- Wails Windows x64 发布版和按 VS Code 预启动任务执行的 debug 构建通过，分别生成 CAF.exe 与 CAF-debug.exe；调试版包含 DWARF 和 all=-N -l。
- 实际生成的 JS 指向 window.go.core.Core，页面需要的 GetStatus、LoadNativeLibrary、BeginLogin 均存在；Startup/Shutdown 不作为 JS 方法暴露。
- 构建使用全新 CAF_DATA_DIR，生成绑定没有初始化平台、创建数据目录、提取 DLL 或尝试登录。
- 全局引用测试验证 core.Platform、页面方法和调用方使用同一个核心与后端；构造独立测试实例不会自动覆盖全局入口。
- 实际桌面 BeginLogin 仍明确返回 ErrLoginNotImplemented，不提取 DLL、不创建会话、不假报登录成功；假后端仅验证调用路由。

原来基线 main.go 的参数/类型错误和登录转发测试失败已修复，不通过删除错误/并发场景掩盖失败。
旧 Service 与桌面 App 的测试迁为同一 Core 的生命周期、状态、重试、全局入口和上下文测试。

## 原生资源未变

- Lagrange 子模块仍为 20c2ba079b5ee67156bd71419ed54ea61a24e91f，工作区未修改。
- DLL 没有移动或重新编译，SHA-256 仍为 2cb457a82c6a002d606502ea322887626bca6cf673ef07c71c76b2bf5b26492c。
- 不包含 PDB，不调用 Release/FreeLibrary；Windows 库仍只支持当前 win-x64 目标。

## 未实测与未实现

- Linux 宿主无法实际运行 Windows GUI、Delve F5 或 Windows DLL；这些检查是交叉编译，不是实机运行。
- Windows PowerShell 5.1、Windows NativeAOT 重编和 Android 未执行；现有脚本由 Linux PowerShell 7.6.7 回归验证。
- QQ 参数封送、二维码解析、凭据持久化、登录/消息收发与 Agent 业务没有实现。
- Linux/macOS 图形后端与其原生库未验证，Android 仍是占位。

补丁只包含源码、文档与测试，不包含生成的绑定、EXE、测试产物、账号数据或依赖目录。
build/dev-data 是开发运行数据，不能当作可任意删除的构建产物；本次没有删除或迁移旧数据。
