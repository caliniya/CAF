package Memory

//表示某一段记忆,
type Recollection struct {
	Name string

	data map[*description]Memory
}

//对记忆的一段描述,我们主要进行模糊搜索
type description []string

var Buildin description = description{
	"用户",
	"地址",
	"爱好",
}

func (r *Recollection) Search() {

}
