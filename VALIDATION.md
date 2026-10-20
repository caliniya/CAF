# 验证记录

2026-10-20，Linux x64 环境；Go 1.27.3、Wails CLI v2.17.0、Node v18.20.4。

## 已通过

- `go mod verify`：模块校验通过；`go mod tidy -diff` 无变化。
- `gofmt`：全部 Go 源文件已格式化。
- `go test -tags bindings -count=1 ./...`：14 个 Go 测试通过。
- `go test -race -tags bindings -count=1 ./...`：通过。
- `go vet -tags bindings ./...`：通过。
- 前端 `.mjs` 文件的 `node --check` 及 `node --test tests/frontend.test.mjs`：4 个测试通过。
- `wails build -platform windows/amd64`：生成 `build/bin/CAF.exe`；确认为 Windows x64 GUI PE。构建过程成功生成 Go/JS 绑定，未执行 npm 安装或前端打包命令。
- Windows x64 的 `cmd/nativecheck` 和原生加载测试程序交叉编译通过；`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet -tags production ./...` 通过。
- 内置 DLL 与旧 CAF 工程的文件 SHA-256 相同；静态检查确认 6 个需要的导出存在，文件不是 Git LFS 占位符。
- DLL 提取测试覆盖完整内容校验、缓存复用、损坏修复、并发提取，以及带空格/中文的路径。
- Linux 下实际运行 `cmd/nativecheck`，正确报“未实现该平台”，且没有提取 Windows DLL。
- 对固定子模块提交的干净导出副本执行 `git apply --check`，NativeAPI 兼容补丁可应用；子模块本身保持干净。

## 尚未验证 / 尚未实现

- 当前没有 Windows 运行环境：**没有实际打开 Wails 窗口或执行 Windows DLL 装载测试**。Windows 专用测试已写好，`test.ps1` 会在目标 Windows x64 电脑上运行它。
- PowerShell 脚本未在 Windows 上执行；`scripts/build-native.ps1` 的 Windows NativeAOT 发布也未运行。本次直接复用已有 DLL，而非重新编译。
- 没有执行 Initialize/Start；没有 QQ 登录、消息收发、模型接入或 Agent 工具执行测试。这些业务尚未实现，不能把加载成功当作登录成功。
- 未构建 Linux/macOS GUI；本环境不具备对应 GTK/WebKit 开发库。非 Windows 原生加载及 Android 仅预留结构，没有可运行实现。

`build/bin/`、`build/checks/` 和生成的 JS 绑定均为可再生构建产物，不纳入初始源码提交。
