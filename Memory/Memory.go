package Memory

import (
	"context"
	"time"

	netclient "github.com/caliniya/caf/NetClient"
)

//表示一个记忆单元
type Memory struct {
	data string
	date string
}

var memoryClock = netclient.NewTimeClient("")

// MemoryTimestamp 返回 UTC 时间及来源；在线取时失败时明确标记本机回退。
// 需要追溯的记忆应保留整个结果，避免只保存日期而丢失时间来源。
func MemoryTimestamp(ctx context.Context) netclient.TimeSample {
	return memoryClock.Now(ctx)
}

// MemoryDate 获取当前 UTC 日期，使用 RFC3339Nano 格式，最多等待 4 秒。
func MemoryDate() string {
	return MemoryTimestamp(context.Background()).UTC.Format(time.RFC3339Nano)
}
