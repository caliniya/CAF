# 项目说明

CAF 是一个 Go 桌面 Agent 框架，计划支持 AI 对话与工具调用。

- core：保存全局核心、状态和平台引用，统一调度业务。
- client：负责 HTTP/WS 通信，目前占位。
- pre：解析外部数据并转换格式，目前占位，不直接执行工具。
- platform：只定义平台接口和公共数据。
- backend：提供各平台实现，包含窗口壳和原生库细节。

QQ 只保留 QQPlatform 接口和 DesktopQQ 占位实现，由启动入口注入 core。
Lagrange 子模块及 DLL 保留但不加载，WSL 暂不需要 Linux .so。
桌面优先，Android 占位，数据目录保持便携。
