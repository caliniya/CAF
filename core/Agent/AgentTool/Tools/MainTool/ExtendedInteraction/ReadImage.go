package ExtendedInteraction

import (
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type Kit struct{ agenttool.ToolKitMeta }

func (k *Kit) Meta() agenttool.ToolKitMeta         { return k.ToolKitMeta }
func (k *Kit) ListName() map[string]agenttool.Tool { return k.Tools }
func (k *Kit) List() []agenttool.Tool              { return []agenttool.Tool{k.Tools[agenttool.ReadImageID]} }
func New() *Kit {
	tool := &ReadImage{}
	tool.Load()
	return &Kit{agenttool.ToolKitMeta{Name: "扩展交互工具", Description: "向模型提供图像等交互内容", Enabled: true, Tools: map[string]agenttool.Tool{tool.ID: tool}}}
}

type ReadImage struct{ agenttool.ToolMeta }

func (t *ReadImage) Meta() *agenttool.ToolMeta { return &t.ToolMeta }
func (t *ReadImage) Load() {
	t.ToolMeta = agenttool.ToolMeta{
		ID: agenttool.ReadImageID, Name: "读取图片", Permission: agenttool.PermissionReadOnly, Enabled: true,
		DisplayType: "image", DisplayContent: "arguments.source",
		Description: "查看一张 PNG 或 JPEG 图片。source 填当前对话显示的 attachment- 编号，或工作区内的相对路径。用户在本轮上传的图片已直接作为视觉输入，无需调用本工具。需要查看历史图片或新的工作区图片时使用。",
		Parameters: map[string]any{
			"type": "object", "properties": map[string]any{"source": map[string]any{"type": "string", "minLength": 1, "description": "图片附件编号或工作区相对路径"}},
			"required": []string{"source"}, "additionalProperties": false,
		},
	}
}
func (t *ReadImage) Fun(ctx *agenttool.ToolContext, args map[string]any) (string, error) {
	source, _ := args["source"].(string)
	source = strings.TrimSpace(source)
	if source == "" {
		return "", errors.New("请指定图片附件编号或工作区相对路径")
	}
	if ctx == nil || ctx.ReadImage == nil {
		return "", errors.New("当前宿主不支持读取图片")
	}
	return ctx.ReadImage(ctx.Context, source)
}
