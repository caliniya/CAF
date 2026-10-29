package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	netclient "github.com/caliniya/caf/NetClient"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

func (c *Core) StartLogin() (int, error) {
	if c == nil || c.QQPlatform == nil {
		return 0, errors.New("QQ 平台尚未初始化")
	}

	return c.QQPlatform.StartLogin(), nil
}

// LoadConfigs 供前端调用：读取配置文件并返回解析后的结果。
func (c *Core) LoadConfigs() (*Configs, error) {
	if c == nil || c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}

	if err := LoadConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}

// configAt 校验前置状态并返回第 index 个配置的指针。
func (c *Core) configAt(index int) (*Config, error) {
	if c == nil || c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("配置尚未加载")
	}

	all := *c.Configs.ALLConfig
	if index < 0 || index >= len(all) {
		return nil, fmt.Errorf("配置下标越界：%d（共 %d 项）", index, len(all))
	}
	return &all[index], nil
}

// SelectConfig 供前端调用：启用第 index 个配置（提供商，从 0 开始）并写回配置文件。
// 启用模型随配置走：切换提供商后沿用该配置自己记住的 EnabledModel。
func (c *Core) SelectConfig(index int) (*Configs, error) {
	selected, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousConfig := c.Configs.EnablesConfig
	c.Configs.EnablesConfig = selected

	if err := SaveConfig(); err != nil {
		c.Configs.EnablesConfig = previousConfig
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}

// SelectModel 供前端调用：在当前启用的配置里选定要使用的模型，
// 记录到该配置的 EnabledModel 字段并写回配置文件。
func (c *Core) SelectModel(model string) (*Configs, error) {
	if c == nil || c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil || c.Configs.ALLConfig == nil {
		return nil, errors.New("配置尚未加载")
	}
	if c.Configs.EnablesConfig == nil {
		return nil, errors.New("尚未选择配置（提供商），请先选择配置")
	}
	if !slices.ContainsFunc(c.Configs.EnablesConfig.Model, func(item apiImpl.Model) bool {
		return item.ID == model
	}) {
		return nil, fmt.Errorf("模型 %q 不在当前配置的模型列表中", model)
	}

	previousModel := c.Configs.EnablesConfig.EnabledModel
	c.Configs.EnablesConfig.EnabledModel = model

	if err := SaveConfig(); err != nil {
		c.Configs.EnablesConfig.EnabledModel = previousModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}

type ModelProbeResult struct {
	Models        []apiImpl.Model `json:"models"`
	ServerAddress string          `json:"serverAddress"`
}

// ProbeModels 查询模型并探测 Chat Completions 请求地址。
func (c *Core) ProbeModels(serverAddress, apiKey string) (*ModelProbeResult, error) {
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return nil, errors.New("API 地址不能为空")
	}
	client := netclient.NewAPIClient(netclient.Options{
		ServerAddress: serverAddress,
		APIKey:        apiKey,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	endpoint, err := client.ProbeChatEndpoint(ctx)
	if err != nil {
		return nil, err
	}
	models, _ := client.GetModels(ctx)
	if models == nil {
		models = []apiImpl.Model{}
	}
	return &ModelProbeResult{Models: models, ServerAddress: endpoint}, nil
}

// AddModelConfig 保存新的 API 配置和模型列表，并将其设为当前启用配置。
func (c *Core) AddModelConfig(name, serverAddress, protocol, apiKey string, models []apiImpl.Model) (*Configs, error) {
	if c == nil || c.Paths == nil {
		return nil, errors.New("路径尚未初始化")
	}
	if c.Configs == nil {
		return nil, errors.New("配置尚未加载")
	}
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return nil, errors.New("API 地址不能为空")
	}
	if protocol == "" {
		protocol = "chat-completions"
	}
	if protocol != "chat-completions" {
		return nil, fmt.Errorf("不支持的协议格式：%s", protocol)
	}
	cleanModels := make([]apiImpl.Model, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		if model.Name == "" {
			model.Name = model.ID
		}
		cleanModels = append(cleanModels, model)
	}
	if len(cleanModels) == 0 {
		return nil, errors.New("至少需要一个模型")
	}

	entry := Config{
		Name:          strings.TrimSpace(name),
		ServerAddress: serverAddress,
		Protocol:      protocol,
		APIKey:        apiKey,
		Model:         cleanModels,
		EnabledModel:  cleanModels[0].ID,
	}
	if c.Configs.ALLConfig == nil {
		all := []Config{}
		c.Configs.ALLConfig = &all
	}
	previousEnabled := c.Configs.EnablesConfig
	*c.Configs.ALLConfig = append(*c.Configs.ALLConfig, entry)
	c.Configs.EnablesConfig = &(*c.Configs.ALLConfig)[len(*c.Configs.ALLConfig)-1]
	if err := SaveConfig(); err != nil {
		*c.Configs.ALLConfig = (*c.Configs.ALLConfig)[:len(*c.Configs.ALLConfig)-1]
		c.Configs.EnablesConfig = previousEnabled
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}
	return c.Configs, nil
}

// RefreshModelConfig 更新已有配置探测到的聊天地址和模型列表。
func (c *Core) RefreshModelConfig(index int, serverAddress string, models []apiImpl.Model) (*Configs, error) {
	entry, err := c.configAt(index)
	if err != nil {
		return nil, err
	}
	serverAddress = strings.TrimSpace(serverAddress)
	if serverAddress == "" {
		return nil, errors.New("API 地址不能为空")
	}
	cleanModels := make([]apiImpl.Model, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		if model.Name == "" {
			model.Name = model.ID
		}
		cleanModels = append(cleanModels, model)
	}
	if len(cleanModels) == 0 {
		return nil, errors.New("探测结果中没有可用模型，原配置未更改")
	}

	previousAddress := entry.ServerAddress
	previousModels := entry.Model
	previousEnabledModel := entry.EnabledModel
	entry.ServerAddress = serverAddress
	entry.Model = cleanModels
	if !slices.ContainsFunc(cleanModels, func(model apiImpl.Model) bool {
		return model.ID == entry.EnabledModel
	}) {
		entry.EnabledModel = cleanModels[0].ID
	}
	if err := SaveConfig(); err != nil {
		entry.ServerAddress = previousAddress
		entry.Model = previousModels
		entry.EnabledModel = previousEnabledModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}
	return c.Configs, nil
}

// RenameConfig 供前端调用：修改第 index 个配置的名称并写回配置文件。
// 名称首尾空白会被去掉；留空表示不命名，前端展示时回退为服务地址。
func (c *Core) RenameConfig(index int, name string) (*Configs, error) {
	entry, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousName := entry.Name
	entry.Name = strings.TrimSpace(name)

	if err := SaveConfig(); err != nil {
		entry.Name = previousName
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}
