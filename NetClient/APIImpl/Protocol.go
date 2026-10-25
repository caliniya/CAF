package apiImpl

// 某种接口
type Protocol interface {
	// EncodeRequest 把内部通用请求编码为线格式 JSON 请求体（纯文本），
	EncodeRequest(req ChatRequest) (string, error)

	// DecodeEvent 把客户端收到的一行纯文本（SSE 数据载荷，已剥掉
	// "data: " 分帧前缀）解码为 0..n 个内部增量。
	// 流结束标记（如 "[DONE]"）由协议识别，返回 Done=true 的 Delta；
	// 无法识别的载荷返回空切片和 nil 错误（跳过），格式错误才返回 error。
	DecodeEvent(text string) ([]Delta, error)
}
