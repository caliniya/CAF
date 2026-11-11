package Memory

import "github.com/caliniya/caf/core/Backend"

// 表示一个记忆单元
type Memory struct {
	data string //当前版本数据
	date string //最后修改日期

}

func MemoryDate() string {
	return Backend.Platform.APIClient.TimeClient.Now(nil)
}
