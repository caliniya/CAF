//go:build windows && amd64

package native

type ByteArrayNative struct {
	Length int32   // 字节数，对应 C# 的 int
	Data   uintptr // 数据的内存地址，对应 C# 的 IntPtr
}

type BotQrCodeEventStruct struct {
	Url   ByteArrayNative // URL 的 UTF-8 字节描述
	Image ByteArrayNative // 二维码图片的字节描述
}
