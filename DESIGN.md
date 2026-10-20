# 项目说明

CAF 将 AI 对话、工具调用和 QQ 消息接入整合到一个桌面程序。

界面使用 Wails，核心使用 Go，Lagrange NativeAPI 通过 goinvoke 内置调用。界面只调用核心服务，不在页面里直接处理 DLL 地址。

当前只做工程骨架和原生库加载入口；Agent、登录和消息业务后续再写。优先 Windows 桌面，Android 暂时占位，数据尽量随程序目录迁移。
