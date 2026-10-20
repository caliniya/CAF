# 集中测试目录

全部自有 Go 测试使用 `package tests`，通过公开接口测试业务包；没有独立测试 Go module，也不在业务目录留下 `*_test.go`。

| 文件 | 内容 |
| --- | --- |
| `helpers_test.go` | 可注入的假后端与测试工厂，不调用真实登录 |
| `runtime_test.go` | 路径所有权、显式初始化、后端注入、错误重试、取消、关闭和并发 |
| `service_test.go` | 应用服务与统一 Runtime 的协作 |
| `backend_test.go` | 真正桌面后端的路径、生命周期、未实现登录，以及 UI 注入/关闭 |
| `desktop_test.go` | 界面门面的构造与启动，不打开窗口或登录 |
| `paths_test.go` | 桌面便携路径、覆盖配置和非法目录 |
| `natives_test.go` | 通过公开 Extract 检查 DLL、来源校验、缓存、损坏修复和并发 |
| `lagrange_windows_test.go` | 仅 Windows x64；子进程加载 DLL，解析导出，不调用 Initialize/Start |
| `lagrange_other_test.go` | 其他平台明确拒绝装载，且不创建缓存 |
| `frontend.test.mjs` | 桌面绑定命名空间、调用、状态、登录错误转发 |

从根目录运行 `go test -count=1 ./tests`，或在这里运行 `go test -count=1`。目录中不导入 Wails 窗口包，因此无需 GTK。`test.ps1` 另用 `bindings` 标签对包含窗口入口的整个工程执行 vet。

原生资源记录从 `../platforms/desktop/natives/PROVENANCE.json` 读取。测试产生的文件使用临时目录，不修改项目 DLL。Windows 原生加载测试用子进程释放文件锁，不尝试卸载 NativeAOT。

集中测试时，业务覆盖率需要显式选取目标包：

```powershell
go test -count=1 -coverpkg=./core/...,./platforms/desktop,./platforms/desktop/native,./platforms/desktop/natives ./tests
```

核心调用测试通过假后端验证注入和路由；真正桌面后端的登录方法必须返回 `platform.ErrLoginNotImplemented`，不会把假后端的成功当作真实登录已实现。
