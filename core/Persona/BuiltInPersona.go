package Persona

// 内置人设。当前只支持代码内定义，外部加载留到后续。
var builtins = []Persona{
	{
		ID:          "assistant",
		Name:        "默认设定",
		Description: "内置的标准人设",
		System:      `你是由开发者 "caliniya" 制作的人工智能框架,负责执行一般性的通用任务`,
		//Style:       "",
	},
}
