# CAF

[![CI](https://github.com/caliniya/CAF/actions/workflows/ci.yml/badge.svg)](https://github.com/caliniya/CAF/actions/workflows/ci.yml)

# caliniya's agent frame

CAF 是 Go/Wails 制作的 Agent 框架软件。

目前支持 OneBot 11 反向 WebSocket基本集成
未来计划实现内置编辑器
计划实现插件等扩展功能

开发辅助脚本放在 [tool](tool/README.md)，包括 WSL 中固定 Windows 主机名的配置工具。

QQ 扩展的主动消息默认宽度为 1，每条符合触发条件的群消息单独判断；判断和回复期间的消息逐条排队，主动发言冷却期间跳过。开启群聊上下文时会附带附近所有成员的消息，白名单只决定哪些发送者能触发模型。引用原文独立于白名单和上下文隔离，优先使用消息缓存，缺失时通过已有 OneBot 连接调用 `get_msg`；原文无法取回时会明确标记。

扩展页面的“上下文管理”提供清除和删除。清除归档旧对话并保留工作区；删除会取消旧任务，移除对话、归档历史和 `Root/InternalWorkspaces` 下对应的整个工作区。两者都保留同一 QQ 用户共享的个人长期记忆。QQ 中可使用 `/扩展名称 clear`，或 `/扩展名称 delete` 查看删除确认指令；agent 上下文在指令后加 `agent`。

主动判断复用模型客户端和 HTTP 连接，但每次判断仍是独立的模型请求，决定回复后再生成正式回复。判断请求不携带完整对话历史或工具声明，仍需为发送的人设、附近消息、引用和模型输出计费；实际费用取决于模型价格、消息量和上下文长度。
