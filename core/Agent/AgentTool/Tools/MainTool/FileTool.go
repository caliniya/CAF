package maintool

import agenttool "github.com/caliniya/caf/core/Agent/AgentTool"

type FileTools struct {
}

type ReadFile struct {
	Tool agenttool.ToolMeta
}

func (t *ReadFile) Load() {
	t.Tool.Name = "ReadFile"
	t.Tool.Description = "读取文件,返回文件的字符串数据"
}
