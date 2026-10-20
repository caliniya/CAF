# 项目说明

CAF 计划把 AI 对话、工具调用和 QQ 接入放进一个 Go 桌面程序。

业务、状态和生命周期统一放在 core；core.Platform 是唯一的全局核心入口。
各平台启动时提供接口实现，核心调用接口，不关心 DLL 或目录的具体实现。
Wails 直接绑定同一个核心对象，不再另设 desktop.App 和 Service 转发。

当前只有工程骨架与原生库装载；真实登录、消息和 Agent 业务尚未实现。
优先 Windows x64，Android 只占位，数据目录保持便携。
