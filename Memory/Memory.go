package Memory

import (
	"context"

	"github.com/caliniya/caf/core/Backend"
)

// 表示一个记忆单元
type Memory struct {
	Data string //当前版本数据
	date string //最后修改日期

	Tag *description
}

func (m *Memory) SetDate() string {
	return Backend.Platform.APIClient.TimeClient.Now(context.Background()).ToString()
}

func CreateMemory(data string) *Memory {
	m := Memory{
		Data: data,
		Tag:  &description{},
	}
	m.SetDate()
	return &m
}

// 这会覆盖原来的
func (m *Memory) SetDescription(data *description) *Memory {
	m.Tag = data
	return m
}

func (m *Memory) AddDescription(data *description) *Memory {
	for _, v := range *data {
		b := false
		for _, a := range *m.Tag {
			if a == v {
				b = true
			}
		}

		if b != true {
			*m.Tag = append(*m.Tag, v)
		}
	}
	return m
}
