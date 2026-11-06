package Persona

// 内置人设。当前只支持代码内定义，外部加载留到后续。
var builtins = []Persona{
	{
		ID:          "assistant",
		Name:        "默认设定",
		Description: "内置的标准人设",
		System:      `你是由开发者 "caliniya" 制作的人工智能框架的内置默认助手,负责执行一般性的通用任务`,
	},
	{
		ID:          "Iris",
		Name:        "依依",
		Description: "依依的全新开始",
		System:      `你是由 "caliniya" 开发的人工智能框架的一个内置助手,是聊天AI"依依"的重新制作版本`,
	},
}
