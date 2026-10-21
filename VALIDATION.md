# 结构变更验证记录

2026-10-21，基于最新 CAF 提交 bc845bf；Linux x64 环境，Go 1.27.3、Wails CLI v2.17.0、PowerShell 7.6.7、Node v18.20.4。

## 已通过

- test.ps1：42 个 Go 测试、整个工程 bindings 标签 vet、7 次开发脚本模拟与 VS Code 配置检查、7 个前端测试。
- go test -race -count=1 ./tests：通过。
- go test -tags bindings -count=1 ./...：全部新包与无窗口工程编译、测试通过。
- Windows x64 的 bindings 交叉编译、测试程序编译，以及 production/dev 标签 vet。
- build.ps1 的 Windows 发布构建，以及按 VS Code 任务参数执行的 Windows debug 构建通过。
- 实际生成的 JS 仍调用 window.go.core.Core；模型包含 native.paused，不暴露 DesktopQQ 或 QQPlatform。
- 构建生成绑定时使用全新 CAF_DATA_DIR，没有创建数据目录、提取库或初始化 QQ。
- 发布和 debug EXE 中都能逐字节找到原样 DLL 资源；保留资源不等于装载 DLL。
- 核心、platform、client 与 pre 的依赖检查：不依赖具体 backend、Wails 或 goinvoke。
- 显式注入的 QQ 实现得到调用；未提供或带类型 nil 的 QQ 明确不可用，不隐式借用基础后端。
- DesktopQQ 是单独的稳定占位引用；初始化、上下文与关闭检查仍有效，暂停不影响核心 ready。
- nativecheck 返回 status=paused，指定的全新目录没有创建，说明没有提取或装载本地库。
- Go 模块完整性、gofmt、前端语法与补丁空白检查通过。
- 在 bc845bf 的全新源码副本上应用补丁后，重跑 test.ps1、无窗口全工程测试及 Windows 包构建均通过；DLL 重命名后哈希不变。

## 资源与范围

- Lagrange 子模块仍为 20c2ba079b5ee67156bd71419ed54ea61a24e91f，未改动。
- DLL 仅迁移到 backend/desktop/natives/win-x64，大小 16261632 字节，SHA-256 仍为 2cb457a82c6a002d606502ea322887626bca6cf673ef07c71c76b2bf5b26492c。
- 没有新增或升级依赖；没有改动第三方来源记录的内容，没有重编 DLL，也没有删除数据或缓存。
- client、pre 与 Android 仍只占位，不假称已实现反向 WS、协议解析、工具执行或 Android 后端。
- QQ 只保留占位；默认测试不会装载真实 Windows DLL，提取测试仅使用测试临时目录。

## 未验证的边界

- 实际 Linux production 构建因环境缺少 pkg-config 而停止；GTK/WebKit 与 WSLg 窗口未实测。
- Windows GUI、Delve 断点及 Windows DLL 未在宿主运行；构建和 vet 属于交叉检查。
- NativeAOT 重编脚本只同步路径并做语法与目标路径检查，没有实际执行重编。
- 没有验证或恢复真实 QQ 登录、消息收发和签名服务，也没有实现完整 Agent 业务。
