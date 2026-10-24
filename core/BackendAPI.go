package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
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
// 切换提供商后，若先前启用的模型不在新配置的模型列表中，模型选择会被清空。
func (c *Core) SelectConfig(index int) (*Configs, error) {
	selected, err := c.configAt(index)
	if err != nil {
		return nil, err
	}

	previousConfig := c.Configs.EnablesConfig
	previousModel := c.Configs.EnabledModel

	c.Configs.EnablesConfig = selected
	if previousModel != "" && !slices.Contains(selected.Model, previousModel) {
		c.Configs.EnabledModel = ""
	}

	if err := SaveConfig(); err != nil {
		c.Configs.EnablesConfig = previousConfig
		c.Configs.EnabledModel = previousModel
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return c.Configs, nil
}

// SelectModel 供前端调用：在当前启用的配置里选定要使用的模型，并写回配置文件。
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
	if !slices.Contains(c.Configs.EnablesConfig.Model, model) {
		return nil, fmt.Errorf("模型 %q 不在当前配置的模型列表中", model)
	}

	previousModel := c.Configs.EnabledModel
	c.Configs.EnabledModel = model

	if err := SaveConfig(); err != nil {
		c.Configs.EnabledModel = previousModel
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
