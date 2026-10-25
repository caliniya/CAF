package apiImpl

// Protocol 是协议抽象：内部通用结构 ↔ 纯文本 的双向编解码。
//
// 职责边界：
//   - 只做文本 ↔ 结构的转换，不感知 URL、HTTP、认证——API 地址存在客户端
//     配置（netclient.Options）里，请求头与分帧由客户端负责；
//   - 实例按流创建，可以持有跨行累积状态（例如 Chat Completions 的
//     tool_call 分片需要聚合到完整才输出）。
type Protocol interface {
	// EncodeRequest 把内部通用请求编码为线格式 JSON 请求体（纯文本），
	// 交给 API 客户端发送。字段映射在这里发生：
	// Thinking → 各家的思考参数、Tools → 各家的工具定义等。
	EncodeRequest(req ChatRequest) (string, error)

	// DecodeEvent 把客户端收到的一行纯文本（SSE 数据载荷，已剥掉
	// "data: " 分帧前缀）解码为 0..n 个内部增量。
	// 流结束标记（如 "[DONE]"）由协议识别，返回 Done=true 的 Delta；
	// 无法识别的载荷返回空切片和 nil 错误（跳过），格式错误才返回 error。
	DecodeEvent(text string) ([]Delta, error)
}
