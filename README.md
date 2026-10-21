# CAF

CAF 是 Go/Wails Agent 框架工程。

本轮只清理验证阶段的演示设计，**没有实现新的 QQ 接口、平台注入或业务架构**。
按确认的范围，QQ 调用封装、配置/签名封装、登录状态机和相应测试也已移除。
当前只会打开空白桌面窗口，不能登录 QQ 或发送消息；这是这次清理的预期结果。

## 保留的内容

- Go module、固定的 Wails / LagrangeGo 依赖与许可文件。
- main.go、Wails 空窗口、空白 frontend/index.html、图标与 Windows 构建资源。
- core、core/platform、backend、client、pre 的目录骨架，以及 Android/Agent 占位。
- 已有的纯路径解析工具、开发/构建脚本和 VS Code 配置。
- 旧 C# DLL、来源记录和历史编译补丁，仅归档，不嵌入或加载。

```powershell
.\dev.ps1       # 空白窗口
.\test.ps1      # 路径与开发脚本检查
.\build.ps1     # Windows x64 构建
```

Go 版本要求仍为 1.26+；Wails 固定 v2.17.0，LagrangeGo 固定到 go.mod 中的原版本。
LagrangeGo 通过 backend/desktop/dependencies.go 中的 caf_deps 构建标签暂时保留，避免 Wails 自动执行 go mod tidy 时删除尚未使用的依赖。
普通构建不会编译或初始化该占位文件中的 SDK；正式的桌面 QQ 后端开始导入 SDK 后，即可删除这个占位文件。
Linux 图形构建仍需要宿主的 GTK/WebKit 等环境，Windows 窗口需要 WebView2。

本轮没有删除或迁移 data/、build/dev-data/ 中的配置、设备、二维码或凭据，空窗口也不会读取它们。
此前已验证的完整代码保留在 Git 提交 c922bf5，可以按需查阅，不必从旧补丁重新恢复整套演示架构。

清理补丁说明见 MIGRATION.md，剩余检查见 tests/README.md，第三方来源见 THIRD_PARTY.md。
