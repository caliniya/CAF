package core

import (
	"errors"
	"fmt"
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

// SelectConfig 供前端调用：启用第 index 个配置（提供商及其模型列表，从 0 开始），
// 并把选择结果写回配置文件。
func (c *Core) SelectConfig(index int) (*Config, error) {
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

	selected := &all[index]
	c.Configs.EnablesConfig = selected

	if err := SaveConfig(); err != nil {
		c.Configs.EnablesConfig = nil
		return nil, fmt.Errorf("写入配置文件失败（%s）：%w", c.Paths.ConfigFile, err)
	}

	return selected, nil
}
