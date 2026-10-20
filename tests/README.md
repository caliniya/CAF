# 集中测试目录

全部自有 Go 测试使用 package tests 和公开接口；没有单独测试 module，也不在业务目录放测试文件。

| 文件 | 内容 |
| --- | --- |
| helpers_test.go | 假平台及独立核心构造，不调用真实 QQ 登录 |
| core_test.go | 构造、路径副本、初始化、错误重试、关闭和并发 |
| core_ui_test.go | 页面直接调用核心、全局同一实例、上下文取消、带类型 nil、缺失 QQ 能力和启动重试 |
| backend_test.go | 桌面基础/QQ 两个接口，目录不可变、明确未实现登录、关闭保护 |
| desktop_test.go | 真实桌面后端交给核心启动，不开窗口、不加载或登录 |
| paths_test.go | 桌面便携路径、相对覆盖和非法路径 |
| natives_test.go | DLL 提取、来源校验、缓存、损坏修复和并发 |
| lagrange_windows_test.go | 仅 Windows x64；子进程加载 DLL，不调用 Initialize/Start |
| lagrange_other_test.go | 非支持平台明确报错，且不创建缓存 |
| frontend.test.mjs | core.Core 命名空间、转发、状态和登录错误；拒绝旧 desktop.App 命名空间 |
| development.test.ps1 | 7 次模拟开发脚本运行和 VS Code 配置，无窗口、不依赖 Pester |

根目录运行：

```powershell
.\test.ps1
go test -count=1 ./tests
go test -race -count=1 ./tests
go test -tags bindings -count=1 ./...
node --test tests/frontend.test.mjs
```

核心与后端测试不导入 Wails 窗口包，无需 GTK。整个工程在无窗口环境用 bindings 标签做检查。
全局入口测试只在单个顺序测试里模拟 main 装配，并在结束后恢复全局变量；其他测试使用独立 Core，不修改全局状态。
假平台登录返回成功只验证调用路由；真实桌面登录必须明确返回 ErrLoginNotImplemented。
Windows 原生加载测试用子进程释放文件锁，不尝试卸载 NativeAOT。

覆盖率可用：

```powershell
go test -count=1 -coverpkg=./core/...,./platforms/desktop,./platforms/desktop/native,./platforms/desktop/natives ./tests
```

Windows 窗口、VS Code/Delve 断点和真实登录仍需在相应环境单独验证。
