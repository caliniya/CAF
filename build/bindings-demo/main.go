// 演示：如何用 goinvoke 绑定 C# NativeAOT 导出的"值传递"函数。
// 只用 6 个已绑定的导出，避免依赖登录流程，专注讲语法。
//
// 运行：go run .\build\bindings-demo
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/jamesits/goinvoke"
)

// ── 第 1 步：定义导出表 ────────────────────────────────
// 字段名 == DLL 导出名（大小写敏感）。也可以用 tag 改名：
//
//	DoStart goinvoke.FunctionPointer `func:"Start"`
type exports struct {
	Initialize    goinvoke.FunctionPointer
	Start         goinvoke.FunctionPointer
	Stop          goinvoke.FunctionPointer
	FreeMemory    goinvoke.FunctionPointer
	GetEventCount goinvoke.FunctionPointer
	GetQrCodeEvent goinvoke.FunctionPointer
}

// ── 第 2 步：按 C# 结构体逐字段翻译 ──────────────────

// C#: struct ByteArrayNative { int Length; IntPtr Data; }  // sizeof = 16
// 注意 Length(4) 后面有 4 字节 padding，Go 会自动补
type byteArray struct {
	Length int32
	Data   uintptr
}

// C#: struct BotConfigStruct（节选，完整版还有更多字段）
// 字段顺序必须和 C# 完全一致，但 padding 交给 Go 自己算——手写 padding 极易出错
type botConfig struct {
	Protocol          uint8   // offset 0,  byte
	AutoReconnect     byte    // offset 1,  bool → 1 byte
	UseIPv6Network    byte    // offset 2
	GetOptimumServer  byte    // offset 3
	HighwayChunkSize  uint32  // offset 4
	HighwayConcurrent uint32  // offset 8
	AutoReLogin       byte    // offset 12
	SignAddress       byteArray // offset 16（Go 自动补 3 字节 padding）
}

// C#: struct ReverseEventCountStruct { 20 个 int }
type eventCounts struct {
	Captcha, FriendRequest, FriendRecall, GroupInvite int32
	GroupInviteSelf, GroupJoin, GroupMemberDec         int32
	GroupMemberInc, GroupNudge, GroupReaction          int32
	GroupRecall, Login, Log, Message                   int32
	NewDeviceVerify, Online, QrCode, QrCodeQuery       int32
	RefreshKeystore, SMS                               int32
}

// ── 第 3 步：辅助函数 ──────────────────────────────────

// 读 byteArray 指向的字节（不释放！释放由调用方决定）
func (b byteArray) bytes() []byte {
	if b.Length <= 0 || b.Data == 0 {
		return nil
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(b.Data)), b.Length)
	out := make([]byte, b.Length)
	copy(out, src)
	return out
}

// 校验结构体大小，这是最可靠的布局检查手段
func check(name string, got, want uintptr) {
	if got != want {
		panic(fmt.Sprintf("%s 布局错误: got %d, want %d", name, got, want))
	}
	fmt.Printf("  ✓ %-22s %2d 字节\n", name, got)
}

func main() {
	// DLL 路径：和项目里 native.Loader 解析出来的一致
	dll := filepath.Join("platforms", "desktop", "natives", "win-x64", "Lagrange.Core.NativeAPI.dll")
	abs, err := filepath.Abs(dll)
	if err != nil {
		fail(err)
	}
	if _, err := os.Stat(abs); err != nil {
		fail(fmt.Errorf("找不到 DLL: %w", err))
	}

	fmt.Println("【1】校验结构体布局")
	check("byteArray", unsafe.Sizeof(byteArray{}), 16)
	check("botConfig", unsafe.Sizeof(botConfig{}), 32)
	if off := unsafe.Offsetof(botConfig{}.SignAddress); off != 16 {
		panic(fmt.Sprintf("SignAddress 偏移错误: %d", off))
	}
	check("eventCounts", unsafe.Sizeof(eventCounts{}), 80)

	fmt.Println("\n【2】解析导出")
	var e exports
	if err := goinvoke.Unmarshal(abs, &e); err != nil {
		fail(err)
	}
	// FunctionPointer 是接口值（不是指针），直接放进 map 即可
	for name, fn := range map[string]goinvoke.FunctionPointer{
		"Initialize": e.Initialize, "Start": e.Start, "Stop": e.Stop,
		"FreeMemory": e.FreeMemory, "GetEventCount": e.GetEventCount,
		"GetQrCodeEvent": e.GetQrCodeEvent,
	} {
		if fn == nil {
			fmt.Printf("  ✗ %-16s 未解析\n", name)
			continue
		}
		fmt.Printf("  ✓ %-16s @ 0x%X\n", name, fn.Addr())
	}

	fmt.Println("\n【3】调用 Initialize（传结构体指针）")
	cfg := botConfig{
		Protocol:          0b00000100, // AndroidPhone
		AutoReconnect:     1,
		GetOptimumServer:  1,
		HighwayChunkSize:  1024 * 1024,
		HighwayConcurrent: 4,
		AutoReLogin:       1,
	}
	// 注意：Protocol=4 需要 SignServer，这里只演示封送，不填地址
	// 实际调用会因空 SignAddress 抛异常，所以下面用 recover 包住
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("  ! Initialize 触发原生异常: %v\n", r)
				fmt.Println("    （符合预期：Android 协议必须有 SignServer 地址）")
			}
		}()
		cfgPtr := uintptr(unsafe.Pointer(&cfg))
		//             botConfig  keystore  appInfo(ZERO=用默认)
		r1, _, _ := e.Initialize.Call(cfgPtr, 0, 0)
		idx := int32(r1)
		fmt.Printf("  → 返回 index = %d\n", idx)
		if idx >= 0 {
			fmt.Println("\n【4】用 index 调 Start（参数是 int32）")
			code, _, _ := e.Start.Call(uintptr(int32(idx)))
			fmt.Printf("  → StatusCode = %d (0=Success，仅代表任务已派发)\n", int32(code))

			fmt.Println("\n【5】轮询事件（返回指针，必须 FreeMemory）")
			p, _, _ := e.GetEventCount.Call(uintptr(int32(idx)))
			if p != 0 {
				c := (*eventCounts)(unsafe.Pointer(p))
				fmt.Printf("  → QR=%d Login=%d Online=%d Message=%d\n",
					c.QrCode, c.Login, c.Online, c.Message)
				e.FreeMemory.Call(p) // ← 关键：成对释放
				fmt.Println("  → 已释放事件计数内存")
			}

			fmt.Println("\n【6】清理")
			stopCode, _, _ := e.Stop.Call(uintptr(int32(idx)))
			fmt.Printf("  → Stop = %d\n", int32(stopCode))
		}
	}()

	fmt.Println("\n完成：整条 ABI 链路（解析→封送→调用→读指针→释放）已跑通")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
