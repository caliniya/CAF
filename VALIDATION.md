# 验证记录

2026-10-20，Linux x64 环境；Go 1.27.3、Wails CLI v2.17.0、Node v18.20.4。

## 本次已通过

- `go mod verify`：模块校验通过；`go mod tidy -diff` 无变化，没有新增依赖。
- 全部自有 Go 文件通过 gofmt；测试集中在 `tests/`，不额外公开私有实现。
- `go test -count=1 ./tests`：32 个 Go 黑盒测试通过。
- `go test -tags bindings -count=1 ./...`：整个工程通过无窗口构建/测试。
- `go test -race -count=1 ./tests`：通过。
- 业务覆盖率命令见 tests/README.md：本次 Linux 参与构建的目标包语句覆盖率为 88.7%，不代表 Windows 原生装载已实际运行。
- `go vet -tags bindings ./...` 和 Windows x64 `go vet -tags production ./...` 均通过。
- 前端文件的 `node --check` 及 `node --test tests/frontend.test.mjs`：5 个测试通过，包括登录未实现错误转发。
- `go list -deps ./core/...` 检查通过：核心不依赖 desktop、Wails、goinvoke 或原生资源包。
- Windows x64 的整个测试程序和 cmd/nativecheck 交叉编译通过，确认为 x64 Console PE。
- `wails build -platform windows/amd64`：成功生成 build/bin/CAF.exe，确认为 Windows x64 GUI PE。
- 绑定生成时使用全新的 CAF_DATA_DIR，构建后该目录仍不存在：构造与生成绑定没有触发平台初始化或 DLL 提取。
- 生成的 desktop.App 仅包含 GetStatus / LoadNativeLibrary / BeginLogin；Startup、Shutdown 未作为 JS 业务方法暴露。
- 桌面后端的真实 BeginLogin 在测试中明确返回 ErrLoginNotImplemented，不创建原生缓存或尝试登录。注入假后端的成功仅用于验证调用路由。

## 已有资源与保留边界

- DLL 从原 natives/win-x64/ 原样移动到 platforms/desktop/natives/win-x64/；内容与 PROVENANCE.json 的 SHA-256 一致，未重新编译，不包含 PDB。
- 缓存测试仍验证提取内容、路径、损坏修复、复用和并发；NativeAOT 映射不卸载。
- Lagrange 子模块固定提交未改变，未修改其源码；重编时仍在构建副本上应用补丁。

## 尚未验证 / 尚未实现

- 没有 Windows 运行环境：没有实际打开窗口或执行 Windows DLL 装载测试。Windows 测试程序已交叉编译，可在目标机用 test.ps1 执行测试。
- PowerShell 脚本及 Windows NativeAOT 发布没有在 Windows 实机执行。本次复用已有 DLL。
- QQ 配置结构体封送、真实初始化、登录、二维码/在线事件、消息收发和 Agent 业务均未实现。加载库或 BeginLogin 方法存在不等于登录可用。
- Linux/macOS GUI 没有实机验证，当前环境没有 GTK/WebKit 开发库；其他桌面原生后端未实现。Android 仅目录占位。

build/bin/、build/checks/ 和自动生成的绑定是可再生构建产物，不纳入源码补丁。
