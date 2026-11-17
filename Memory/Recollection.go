package Memory

import "strings"

// 表示某一段记忆,
type Recollection struct {
	Name        string            `json:"name"`
	Data        map[string]Memory `json:"data"`
	Description string            `json:"description"`
}

// 从标签和内容搜索
func (r *Recollection) Search(f func(...string), tags ...string) {
	if r == nil || f == nil {
		return
	}
	for _, k := range r.Data {
		k.Tag.Find(f, tags...)
		for _, v := range tags {
			if strings.Contains(k.Data, v) {
				f(k.Data)
			}
		}
	}

}

// 对记忆的一段描述,我们主要进行模糊搜索
type description []string

func (d *description) Find(get func(...string), tags ...string) {
	if d == nil || get == nil {
		return
	}
	for _, v2 := range tags {
		for _, v := range *d {
			if v == v2 {
				get(v)
			}
		}

	}
}
func (r *description) Get(tags ...string) []string {
	var t []string
	r.Find(func(s ...string) {
		t = append(t, s...)
	}, tags...)
	return t
}

func cloneRecollection(r Recollection) Recollection {
	data := make(map[string]Memory, len(r.Data))
	for key, memory := range r.Data {
		data[key] = cloneMemory(memory)
	}
	r.Data = data
	return r
}
