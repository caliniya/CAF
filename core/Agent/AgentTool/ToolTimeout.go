package AgentTool

import (
	"fmt"
	"math"
	"time"
)

const DefaultToolTimeout = 30 * time.Second

// TimeoutFor 按「调用参数 → 工具默认值 → 30 秒」解析执行超时。
// args 来自 JSON 解码，因此数字参数的类型是 float64。
func (m *ToolMeta) TimeoutFor(args map[string]any) (time.Duration, error) {
	timeout := DefaultToolTimeout
	if m == nil {
		return timeout, nil
	}
	if m.Timeout < 0 {
		return 0, fmt.Errorf("工具 %q 的默认超时不能为负数", m.ID)
	}
	if m.Timeout > 0 {
		timeout = m.Timeout
	}
	if m.TimeoutParam == "" {
		return timeout, nil
	}
	value, present := args[m.TimeoutParam]
	if !present {
		return timeout, nil
	}
	seconds, ok := value.(float64)
	if !ok || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, fmt.Errorf("参数 %s 必须是非负数字（单位：秒）", m.TimeoutParam)
	}
	if seconds == 0 {
		return timeout, nil
	}
	// 先检查范围，避免秒数转换成 time.Duration 时溢出或被截断为 0。
	if seconds >= float64(math.MaxInt64)/float64(time.Second) || seconds < 1/float64(time.Second) {
		return 0, fmt.Errorf("参数 %s 超出了可用的超时时间范围", m.TimeoutParam)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
