# CAF

[![CI](https://github.com/caliniya/CAF/actions/workflows/ci.yml/badge.svg)](https://github.com/caliniya/CAF/actions/workflows/ci.yml)

# caliniya's agent frame

CAF 是 Go/Wails 制作的 Agent 框架软件。

目前支持 OneBot 11 反向 WebSocket QQ 聊天，包括白名单、群聊上下文、主动发言、独立 agent 上下文和个人长期记忆。
未来计划实现内置编辑器
计划实现插件等扩展功能

开发辅助脚本放在 [tool](tool/README.md)，包括 WSL 中固定 Windows 主机名的配置工具。

QQ 扩展在客户端页面创建。编辑扩展可调整触发规则、白名单和上下文选项；“对话配置”选择模型、人设、工具和严格模式。允许对话的私聊直接回复；群聊使用关键词触发，关键词留空时使用扩展名称。启用 agent 后，群聊 @ 机器人进入每个人独立的 agent 上下文。

开启上下文隔离时，同一人的私聊和群聊共用上下文；关闭时各群共享，私聊按人分开。长期记忆按机器人 QQ 和发送者 QQ 保存，普通聊天和 agent 对话共用个人记忆。每个上下文使用 `Root/InternalWorkspaces` 下的独立目录和自动编辑权限。

“上下文管理”可以清除一条或全部 QQ 上下文。QQ 中也可发送 `/扩展名称 clear`，或 `/扩展名称 clear agent`。清除会取消旧生成和排队消息，旧对话归档，工作区文件和长期记忆保留。模型提问与工作区工具申请通过回复中的 `answer`、`approve` 指令处理；只有原用户在原聊天中才能回答。

主动消息按宽度打包，先用无工具的独立请求判断是否发言；决定不回复的批次不写入聊天历史。开启 QQ 白名单时只打包白名单成员；触发聊天时的附近群聊上下文仍包含所有群成员。默认宽度 20 条、发言间隔 60 秒，均可编辑。
