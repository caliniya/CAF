# 本次迁移：统一核心

补丁基于 `830e209`，文件名 `CAF-unified-core.patch`。
只应用本次补丁；之前的 core/platform、开发目录和 VS Code 补丁已在基线中，不要重复应用。

```powershell
git apply --check .\CAF-unified-core.patch
git apply .\CAF-unified-core.patch
.\test.ps1
.\build.ps1
```

有其他本地改动时先保存，检查报错就不要继续强行应用。
本次没有提交或推送代码，没有修改 Lagrange 子模块或 DLL。

主要变化：

- 全局 `core.Platform` 改为 `*core.Core`，main 创建一次并赋值；窗口绑定同一实例，不复制互斥锁。
- 删除 `platforms/desktop/ui.go` 中的 App 和 `core/app/service.go` 中的 Service；业务、状态与生命周期直接在 core 中。
- 窗口函数接收 *core.Core。核心自行处理 Startup/Shutdown 和页面调用。
- 前端由 window.go.desktop.App 改为 window.go.core.Core；已同步修改 bridge 和测试，生成文件由 Wails 重新生成。
- QQ 接口从拼写有误的 QQPlatfrom 整理为 QQPlatform，与 Backend 分开；使用普通 Go 上下文和 error，不把裸 DLL 索引当作可直接使用的登录接口。
- 启动和登录错误正常记录、传递；桌面登录继续明确返回尚未实现，不新增任何实际 QQ 登录。
- 所有自有测试仍在 tests，原 Runtime/Service 测试更新为 Core 与全局同一实例的测试，没有删掉错误、并发和生命周期覆盖。

原先 Go 调用 BeginLogin(ctx) 的位置，改为核心 Startup(ctx) 保存上下文后调用 BeginLogin()，返回 (core.Status,error)。
命令行可先 Init，再调用核心方法；程序入口与命令行检查都已更新。

数据位置、F5 配置和原生库重编脚本不变；开发数据仍在 build/dev-data，旧数据不会被删除。
