package Memory

//表示某一段记忆,
type Recollection struct {
	Name string `json:""`
	data map[*description]Memory
}

func (r *description) Get(tags ...string) []string {
	var t []string
	r.Find(func(s ...string) {
		t = s
	}, tags...)
	return t
}

func (r *Recollection) Search(f func(...string), tags ...string) {
	data := r.data
	for k := range data {
		k.Find(f, tags...)
	}
}

//对记忆的一段描述,我们主要进行模糊搜索
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
