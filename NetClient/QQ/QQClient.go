package QQ

// QQClient 是 QQ 客户端的公共接口，预留给 OneBot 11 反向 WebSocket
// 和 LagrangeGo 等适配器实现。上层收发使用 APIImpl.QQMessage；
// 各适配器负责内部 QQ 协议与外部协议的转换。
//
// 本阶段保持空接口，具体方法在实现适配器时确定。
// OneBot 11 适配器只依赖协议，不按提供方区分收发逻辑。
type QQClient interface{}
