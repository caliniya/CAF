package Memory

import "strings"

// 表示某一段记忆,
type Recollection struct {
	Name        string                  `json:"name"`
	Data        map[*description]Memory `json:"data"`
	Description string
}

// 从标签和内容搜索
func (r *Recollection) Search(f func(...string), tags ...string) {
	data := r.Data
	for k := range data {
		k.Find(f, tags...)
	}
	for _, k := range r.Data {
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
		t = s
	}, tags...)
	return t
}
