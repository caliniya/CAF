package Backend

import (
	"encoding/json"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// modelInfoPrompt 只附带会话所选模型的元数据和本次请求参数。
func modelInfoPrompt(models []apiImpl.Model, modelID, reasoningEffort string) (string, error) {
	info := struct {
		apiImpl.Model
		ReasoningEffort string `json:"reasoning_effort,omitempty"`
	}{
		Model:           apiImpl.Model{ID: modelID},
		ReasoningEffort: reasoningEffort,
	}
	for _, model := range models {
		if model.ID == modelID {
			info.Model = model
			break
		}
	}
	data, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return "以下是当前使用的模型信息和本次请求参数：\n" + string(data), nil
}
