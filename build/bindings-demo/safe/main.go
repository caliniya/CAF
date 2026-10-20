// 演示 2：安全调用模式。
// 相比 demo 1 修正了三个致命问题：
//  1. 用 C 堆（LocalAlloc）分配结构体，不用 Go 栈地址
//  2. ByteArrayNative 传有效指针（哪怕空字符串也要给 1 字节缓冲，绝不留 0）
//  3. 用 recover 无法捕获 native 崩溃——只能靠正确封送避免
//
// 运行：go run .\build\bindings-demo\safe
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/jamesits/goinvoke"
	"golang.org/x/sys/windows"
)

type exports struct {
	Initialize     goinvoke.FunctionPointer
	Start          goinvoke.FunctionPointer
	Stop           goinvoke.FunctionPointer
	FreeMemory     goinvoke.FunctionPointer
	GetEventCount  goinvoke.FunctionPointer
}

// C#: struct ByteArrayNative { int Length; IntPtr Data; }
type byteArray struct {
	Length int32
	Data   uintptr
}

// C#: struct BotConfigStruct —— 只到 SignAddress 为止
type botConfig struct {
	Protocol          uint8
	AutoReconnect     byte
	UseIPv6Network    byte
	GetOptimumServer  byte
	HighwayChunkSize  uint32
	HighwayConcurrent uint32
	AutoReLogin       byte
	SignAddress       byteArray
}

type eventCounts struct {
	Captcha, FriendRequest, FriendRecall, GroupInviteSelf, GroupInvite int32
	GroupJoin, GroupMemberDec, GroupMemberInc, GroupNudge, GroupReaction int32
	GroupRecall, Login, Log, Message, NewDeviceVerify int32
	Online, QrCode, QrCodeQuery, RefreshKeystore, SMS int32
}

// cStr 把 Go 字符串放进 C 堆，返回 ByteArrayNative。
// 关键：即使空字符串也要分配 1 字节，绝不返回 Data=0。
func cStr(s string) (byteArray, uintptr, error) {
	data := []byte(s)
	if len(data) == 0 {
		data = []byte{0} // 哨兵字节，Length 仍报 0
	}
	ptr, err := windows.LocalAlloc(0, uint32(len(data)))
	if err != nil {
		return byteArray{}, 0, err
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	return byteArray{Length: int32(len(s)), Data: ptr}, ptr, nil
}

func main() {
	dll, err := filepath.Abs(filepath.Join("platforms", "desktop", "natives", "win-x64", "Lagrange.Core.NativeAPI.dll"))
	if err != nil {
		fail(err)
	}

	var e exports
	if err := goinvoke.Unmarshal(dll, &e); err != nil {
		fail(err)
	}
	fmt.Println("导出解析完成")

	// ── 准备 SignServer 地址（必需！）──
	// AndroidPhone 协议强制要求 SignServer，空值会导致托管异常穿越 native 边界 = 进程崩溃
	sign := "http://127.0.0.1:8080" // 占位；没有真实签名服务时，下一步仍会失败
	signBA, _, err := cStr(sign)
	if err != nil {
		fail(err)
	}
	defer windows.LocalFree(windows.Handle(signBA.Data))

	cfg := botConfig{
		Protocol:          0b00000100, // AndroidPhone
		AutoReconnect:     1,
		UseIPv6Network:    0,
		GetOptimumServer:  1,
		HighwayChunkSize:  1024 * 1024,
		HighwayConcurrent: 4,
		AutoReLogin:       1,
		SignAddress:       signBA,
	}

	// ── 关键：结构体必须在 C 堆上，不能是 Go 栈地址 ──
	cfgPtr, err := windows.LocalAlloc(0, uint32(unsafe.Sizeof(cfg)))
	if err != nil {
		fail(err)
	}
	defer windows.LocalFree(windows.Handle(cfgPtr))
	*(*botConfig)(unsafe.Pointer(cfgPtr)) = cfg

	fmt.Printf("botConfig size=%d, SignAddress.Data=0x%X\n", unsafe.Sizeof(cfg), cfg.SignAddress.Data)

	// keystore=nil(0) → 走 BotFactory.Create(botConfig, appInfo) 空 keystore 路径
	// appInfo=nil(0) → 用内置 ProtocolToAppInfo 默认值
	idx, _, _ := e.Initialize.Call(cfgPtr, 0, 0)
	fmt.Printf("Initialize → index = %d\n", int32(idx))

	if int32(idx) < 0 {
		fmt.Println("Initialize 失败")
		return
	}
	defer e.Stop.Call(uintptr(int32(idx)))

	code, _, _ := e.Start.Call(uintptr(int32(idx)))
	fmt.Printf("Start → StatusCode = %d (0=Success，仅代表任务派发)\n", int32(code))

	// 轮询事件
	p, _, _ := e.GetEventCount.Call(uintptr(int32(idx)))
	if p != 0 {
		c := (*eventCounts)(unsafe.Pointer(p))
		fmt.Printf("事件计数: QR=%d Login=%d Online=%d\n", c.QrCode, c.Login, c.Online)
		e.FreeMemory.Call(p) // 成对释放
		fmt.Println("已释放事件计数内存")
	}

	fmt.Println("\n如果到这里没有崩溃，说明封送链路是正确的。")
	fmt.Println("（没有真实 SignServer 时，登录会在网络阶段失败，但 ABI 层已通）")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
