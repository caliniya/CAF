# 演示清理验证

2026-10-21，基于 c922bf5。本轮范围是删除演示实现及 QQ 封装，保留工程骨架，不实现新架构。

## 已通过

- test.ps1：3 个纯路径测试、整个工程 bindings vet、7 次开发脚本模拟及 VS Code 配置检查。
- 剩余 Go 测试的 race 检查、全工程 bindings 标签测试、Windows x64 包构建和 production vet。
- 在 c922bf5 的全新副本应用清理补丁，重新运行检查；Wails Windows x64 production/debug 空窗口构建均通过。
- 两种 EXE 的模块信息均不包含 LagrangeGo/goinvoke，不嵌入归档 DLL；模块声明仍为后续开发保留 SDK。
- 全新绑定目录没有生成旧 Core.js，绑定生成过程不初始化指定的数据目录。
- 业务源码不再包含旧 Core/QQ 对象、登录/发消息方法、QQ 配置/签名封装或 DLL 提取入口。
- Go 模块校验、gofmt 和 diff 空白检查；依赖声明、归档 DLL/来源记录与清理前一致。

## 保留与未验证

没有新的 QQ 登录、二维码、发消息接口或后端，因此不再运行旧演示场景测试，也不把之前的验证当作新架构已经实现。
本地配置/凭据不删除、不覆盖、不迁移。为保留后续开发要用的 SDK，没有执行会移除闲置依赖的自动 tidy。
Windows GUI 未在 Windows 实机运行；Linux 图形窗口未运行，当前验证是无窗口检查及 Windows 交叉构建。
