# 项目说明

CAF 是一个 Go/Wails 桌面 Agent 框架，计划支持 AI 对话与工具调用。
业务放 core，接口放 core/platform，具体实现放 backend，client 和 pre 目前占位。
全局入口是 core.Platform，不另设 desktop.App。

桌面 QQ 改用 LagrangeGo，直接通过 Go 方法实现平台接口，不再使用 C# 子模块或本地 FFI。
当前只试验扫码登录、凭据复用和手动群文本；真实签名服务与账号尚未实测。
启动不会自动登录或发消息，令牌与凭据只保存在本机数据目录。
Android 仍占位；旧 DLL 仅归档，不进入运行流程。
