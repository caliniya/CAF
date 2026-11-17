package Memory

import (
	"time"
)

// 表示一个记忆单元
type Memory struct {
	Data       string       `json:"data"`
	MemoryDate string       `json:"date"`
	Tag        *description `json:"description"`
}

func (m *Memory) Date() string {
	m.MemoryDate = time.Now().UTC().Format(time.RFC3339Nano)
	return m.MemoryDate
}

func CreateMemory(data string) *Memory {
	m := Memory{
		Data: data,
		Tag:  &description{},
	}
	m.Date()
	return &m
}

// 这会覆盖原来的
func (m *Memory) SetDescription(data *description) *Memory {
	if data == nil {
		m.Tag = &description{}
	} else {
		tags := append(description{}, (*data)...)
		m.Tag = &tags
	}
	return m
}

func (m *Memory) AddDescription(data *description) *Memory {
	if data == nil {
		return m
	}
	if m.Tag == nil {
		m.Tag = &description{}
	}
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

func cloneMemory(m Memory) Memory {
	if m.Tag != nil {
		tags := append(description{}, (*m.Tag)...)
		m.Tag = &tags
	}
	return m
}
