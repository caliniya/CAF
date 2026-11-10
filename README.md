# CAF
# caliniay's agent frame

CAF 是 Go/Wails 制作的 Agent 框架软件。

首次启动会在程序旁的 `data/config/MainConfig.json` 创建空配置，随后可在“模型设置”中添加供应商和模型。设置 `CAF_DATA_DIR` 可以指定数据目录。

QQ 接入通过 Onebot11QQ 扩展提供，支持 OneBot 11 反向 WebSocket。
未来计划实现内置编辑器
计划实现插件等扩展功能

开发辅助脚本放在 [tool](tool/README.md)，包括 WSL 中固定 Windows 主机名的配置工具。
