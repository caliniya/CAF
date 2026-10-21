// qqcheck 是无窗口的最小试验入口，方便在 WSL 中验证登录，不依赖 GTK/WebKit。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/caliniya/caf/backend/desktop"
	"github.com/caliniya/caf/core"
)

func main() {
	data := flag.String("data-dir", "build/dev-data", "数据目录，相对路径按当前目录解析")
	group := flag.Uint64("group", 0, "可选：登录后发送一条测试消息的群号")
	text := flag.String("text", "", "可选：测试消息文本；不指定则绝不发送")
	flag.Parse()
	if err := run(*data, *group, *text); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(data string, group uint64, text string) error {
	if group > 4294967295 || (group == 0) != (text == "") {
		return errors.New("发送测试必须同时提供有效 -group 和非空 -text")
	}
	directory, err := filepath.Abs(data)
	if err != nil {
		return err
	}
	paths, err := desktop.DiscoverPaths(directory)
	if err != nil {
		return err
	}
	backend := desktop.New()
	engine, err := core.NewCore(paths, backend, backend.QQ())
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	engine.Startup(ctx)
	defer engine.Close()
	if _, err := engine.BeginLogin(); err != nil {
		return err
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	shown, sent := false, false
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			qq := engine.GetStatus().QQ
			if (qq.Phase == "waiting" || qq.Phase == "scanned") && !shown {
				fmt.Println("请在本机打开", filepath.Join(directory, "qq", "qrcode.png"), "并扫码，不要公开二维码。")
				shown = true
			}
			if qq.Phase == "error" {
				return errors.New(qq.Error)
			}
			if qq.Phase == "online" && !sent {
				fmt.Println("QQ 已上线；按 Ctrl+C 关闭会话。")
				if group != 0 {
					if _, err := engine.SendGroupText(uint32(group), text); err != nil {
						return err
					}
					fmt.Println("测试消息发送调用成功，不自动重试。")
				}
				sent = true
			}
		}
	}
}
