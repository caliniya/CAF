package Backend

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	persona "github.com/caliniya/caf/core/Persona"
)

// ExtensionChatConfig 引用宿主模型配置，不在扩展中复制 API 密钥。
type ExtensionChatConfig struct {
	ConfigID        string                    `json:"configId"`
	Model           string                    `json:"model"`
	PersonaID       string                    `json:"personaId"`
	ReasoningEffort string                    `json:"reasoningEffort"`
	ToolConfig      *agenttool.ChatToolConfig `json:"toolConfig"`
}

type QQChatOptions struct {
	TriggerKeywords    []string `json:"triggerKeywords"`
	GroupContext       bool     `json:"groupContext"`
	GroupContextLength int      `json:"groupContextLength"`
	WhitelistEnabled   bool     `json:"whitelistEnabled"`
	Whitelist          []string `json:"whitelist"`
	ContextIsolation   bool     `json:"contextIsolation"`
	ProactiveEnabled   bool     `json:"proactiveEnabled"`
	ProactiveWidth     int      `json:"proactiveWidth"`
	ProactiveCooldown  int      `json:"proactiveCooldown"`
	LongTermMemory     bool     `json:"longTermMemory"`
	AgentEnabled       bool     `json:"agentEnabled"`
	GroupWhitelist     []string `json:"groupWhitelist"`
}

func defaultQQChatOptions() QQChatOptions {
	return QQChatOptions{GroupContext: true, GroupContextLength: 10, ContextIsolation: true, ProactiveWidth: 20, ProactiveCooldown: 60}
}

func copyQQChatOptions(value *QQChatOptions) QQChatOptions {
	if value == nil {
		return defaultQQChatOptions()
	}
	copy := *value
	copy.TriggerKeywords = slices.Clone(value.TriggerKeywords)
	copy.Whitelist = slices.Clone(value.Whitelist)
	copy.GroupWhitelist = slices.Clone(value.GroupWhitelist)
	return copy
}

func parseQQNumbers(value string) ([]string, error) {
	result := []string{}
	for _, field := range strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '，' }) {
		if strings.IndexFunc(field, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, errors.New("QQ 号和群号必须为正整数，用逗号或空格分隔")
		}
		number, err := strconv.ParseInt(field, 10, 64)
		if err != nil || number <= 0 {
			return nil, errors.New("QQ 号和群号必须为正整数")
		}
		field = strconv.FormatInt(number, 10)
		if !slices.Contains(result, field) {
			result = append(result, field)
		}
	}
	if len(result) > 1000 {
		return nil, errors.New("白名单最多包含 1000 个号码")
	}
	return result, nil
}

func parseQQChatOptions(parameters map[string]string, previous *QQChatOptions) (*QQChatOptions, error) {
	options := copyQQChatOptions(previous)
	for _, item := range []struct {
		key    string
		target *bool
	}{
		{"groupContext", &options.GroupContext}, {"whitelistEnabled", &options.WhitelistEnabled},
		{"contextIsolation", &options.ContextIsolation}, {"proactiveEnabled", &options.ProactiveEnabled},
		{"longTermMemory", &options.LongTermMemory}, {"agentEnabled", &options.AgentEnabled},
	} {
		if value, exists := parameters[item.key]; exists {
			if value != "true" && value != "false" {
				return nil, fmt.Errorf("%s 必须为 true 或 false", item.key)
			}
			*item.target = value == "true"
		}
	}
	for _, item := range []struct {
		key              string
		target           *int
		minimum, maximum int
	}{
		{"groupContextLength", &options.GroupContextLength, 1, 100},
		{"proactiveWidth", &options.ProactiveWidth, 1, 100}, {"proactiveCooldown", &options.ProactiveCooldown, 0, 3600},
	} {
		if value, exists := parameters[item.key]; exists {
			number, err := strconv.Atoi(value)
			if err != nil || number < item.minimum || number > item.maximum {
				return nil, fmt.Errorf("%s 必须在 %d 到 %d 之间", item.key, item.minimum, item.maximum)
			}
			*item.target = number
		}
	}
	for _, item := range []struct {
		key    string
		target *[]string
	}{{"whitelist", &options.Whitelist}, {"groupWhitelist", &options.GroupWhitelist}} {
		if value, exists := parameters[item.key]; exists {
			ids, err := parseQQNumbers(value)
			if err != nil {
				return nil, err
			}
			*item.target = ids
		}
	}
	if value, exists := parameters["triggerKeywords"]; exists {
		options.TriggerKeywords = []string{}
		for _, word := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == '\n' || r == '\r' }) {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			if len([]rune(word)) > 100 {
				return nil, errors.New("单个触发关键词最长 100 个字符")
			}
			if !slices.Contains(options.TriggerKeywords, word) {
				options.TriggerKeywords = append(options.TriggerKeywords, word)
			}
		}
		if len(options.TriggerKeywords) > 32 {
			return nil, errors.New("最多设置 32 个触发关键词")
		}
	}
	return &options, nil
}

func (o QQChatOptions) parameters() map[string]string {
	return map[string]string{
		"triggerKeywords": strings.Join(o.TriggerKeywords, ", "), "groupContext": strconv.FormatBool(o.GroupContext),
		"groupContextLength": strconv.Itoa(o.GroupContextLength), "whitelistEnabled": strconv.FormatBool(o.WhitelistEnabled),
		"whitelist": strings.Join(o.Whitelist, ", "), "contextIsolation": strconv.FormatBool(o.ContextIsolation),
		"proactiveEnabled": strconv.FormatBool(o.ProactiveEnabled), "proactiveWidth": strconv.Itoa(o.ProactiveWidth),
		"proactiveCooldown": strconv.Itoa(o.ProactiveCooldown), "longTermMemory": strconv.FormatBool(o.LongTermMemory),
		"agentEnabled": strconv.FormatBool(o.AgentEnabled), "groupWhitelist": strings.Join(o.GroupWhitelist, ", "),
	}
}

func (c *Core) defaultExtensionChatConfig() ExtensionChatConfig {
	strict := false
	config := ExtensionChatConfig{PersonaID: "assistant", ReasoningEffort: "high", ToolConfig: &agenttool.ChatToolConfig{Permission: agenttool.PermissionAutoEdit, StrictMode: &strict}}
	if c.Configs != nil && c.Configs.EnablesConfig != nil {
		config.ConfigID, config.Model = c.Configs.EnablesConfig.ID, c.Configs.EnablesConfig.EnabledModel
	}
	return config
}

func (c *Core) extensionChatConfig(value *ExtensionChatConfig) ExtensionChatConfig {
	if value == nil {
		return c.defaultExtensionChatConfig()
	}
	copy := *value
	copy.ToolConfig = value.ToolConfig.Clone()
	if copy.ToolConfig == nil {
		copy.ToolConfig = &agenttool.ChatToolConfig{}
	}
	copy.ToolConfig.Permission = agenttool.PermissionAutoEdit
	return copy
}

func (c *Core) GetClientExtensionChatConfig(clientID, extensionID string) (*ExtensionChatConfig, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	client, index, err := c.clientExtensionLocked(clientID, extensionID)
	if err != nil {
		return nil, err
	}
	config := c.extensionChatConfig(client.Extensions[index].Chat)
	return &config, nil
}

func (c *Core) UpdateClientExtensionChatConfig(clientID, extensionID string, config ExtensionChatConfig) (*ExtensionChatConfig, error) {
	c.extMu.Lock()
	defer c.extMu.Unlock()
	client, index, err := c.clientExtensionLocked(clientID, extensionID)
	if err != nil {
		return nil, err
	}
	provider, err := c.configByID(config.ConfigID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(provider.Model, func(model api.Model) bool { return model.ID == config.Model }) {
		return nil, errors.New("模型不属于所选配置")
	}
	if persona.Resolve(config.PersonaID).ID != config.PersonaID {
		return nil, errors.New("人设不存在")
	}
	if normalizeReasoningEffort(config.ReasoningEffort) != config.ReasoningEffort {
		return nil, errors.New("无效思考强度")
	}
	config = c.extensionChatConfig(&config)
	config.ToolConfig.AllowedTools = nil
	for setID, set := range config.ToolConfig.Sets {
		available, exists := agenttool.ToolSets[setID]
		if !exists {
			return nil, errors.New("工具集不存在")
		}
		for kitID := range set.Kits {
			if available.Meta().ToolKits[kitID] == nil {
				return nil, errors.New("工具模块不存在")
			}
		}
	}
	config.ToolConfig.Normalize()
	client.Extensions[index].Chat = &config
	if err := c.saveExtensionClientLocked(client); err != nil {
		return nil, err
	}
	c.cancelQQChatRuns(extensionID)
	return &config, nil
}
