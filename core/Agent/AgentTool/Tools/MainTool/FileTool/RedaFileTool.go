package filetool

type ReadFile struct {
	name        string
	description string
}

func (t *ReadFile) Load() {
	t.name = "ReadFile"
	t.description = "读取文件,返回文件的字符串数据"
}

func (t *ReadFile) Name() string {
	return t.name
}

func (t *ReadFile) Description() string {
	return t.description
}

func (t *ReadFile) Fun(arg map[string]any) (string, error) {
	return "", nil
}
