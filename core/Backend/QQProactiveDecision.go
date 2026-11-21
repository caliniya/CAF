package Backend

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var qqDecisionField = regexp.MustCompile(`(?i)["']?\breply\b["']?\s*[:=]\s*["']?(true|ture|false|flase|yes|no|1|0)\b`)
var qqDecisionWord = regexp.MustCompile(`(?i)\b(true|ture|false|flase|yes|no)\b`)

// ParseQQProactiveDecision 兼容布尔值、reply 对象、代码块及常见大小写或拼写差异。
// 缺失判断字段、无法识别或相互矛盾的结果会报错，不默认当作“不回复”。
func ParseQQProactiveDecision(output string) (bool, error) {
	text := strings.TrimSpace(strings.ToLower(output))
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSpace(strings.TrimPrefix(text, "json"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	}
	var value any
	if json.Unmarshal([]byte(text), &value) == nil {
		if object, ok := value.(map[string]any); ok {
			var exists bool
			value, exists = object["reply"]
			if !exists {
				return false, errors.New("缺少 reply 判断字段")
			}
		}
		return qqDecisionValue(value)
	}
	if reply, err := qqDecisionValue(strings.Trim(text, " \t\r\n\"'`*。.!！;；()（）[]")); err == nil {
		return reply, nil
	}
	matches := qqDecisionField.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 && !strings.ContainsAny(text, "{}=") {
		matches = qqDecisionWord.FindAllStringSubmatch(text, -1)
	}
	if len(matches) == 0 {
		return false, errors.New("没有可识别的回复判断")
	}
	var reply bool
	for index, match := range matches {
		current, err := qqDecisionValue(match[1])
		if err != nil || index > 0 && reply != current {
			return false, errors.New("回复判断相互矛盾")
		}
		reply = current
	}
	return reply, nil
}

func qqDecisionValue(value any) (bool, error) {
	switch value := value.(type) {
	case bool:
		return value, nil
	case float64:
		if value == 0 || value == 1 {
			return value == 1, nil
		}
	case string:
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "true", "ture", "yes", "1":
			return true, nil
		case "false", "flase", "no", "0":
			return false, nil
		}
	}
	return false, errors.New("判断值必须表示 true 或 false")
}
