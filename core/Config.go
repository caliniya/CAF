package core

import (
	"encoding/json"
	"fmt"
	"strings"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

type Configs struct {
	ALLConfig     *[]Config `json:"config"`
	EnablesConfig *Config
}

type Config struct {
	ID                string          `json:"ID"`
	Name              string          `json:"Name"`
	ServerAddress     string          `json:"ServerAddress"`
	Protocol          string          `json:"Protocol"`
	APIKey            string          `json:"APIKey"`
	Model             []apiImpl.Model `json:"Model"`
	EnabledModel      string          `json:"EnabledModel"`
	legacyModelFormat bool            `json:"-"`
}

func (c *Config) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID            string          `json:"ID"`
		Name          string          `json:"Name"`
		ServerAddress string          `json:"ServerAddress"`
		Protocol      string          `json:"Protocol"`
		APIKey        string          `json:"APIKey"`
		Model         json.RawMessage `json:"Model"`
		EnabledModel  string          `json:"EnabledModel"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.ID = raw.ID
	c.Name = raw.Name
	c.ServerAddress = raw.ServerAddress
	c.Protocol = raw.Protocol
	if c.Protocol == "" {
		c.Protocol = "chat-completions"
	}
	c.APIKey = raw.APIKey
	c.EnabledModel = raw.EnabledModel
	if len(raw.Model) == 0 || string(raw.Model) == "null" {
		c.Model = nil
		return nil
	}
	if err := json.Unmarshal(raw.Model, &c.Model); err == nil {
		return nil
	}
	var legacy []string
	if err := json.Unmarshal(raw.Model, &legacy); err != nil {
		return fmt.Errorf("解析模型配置失败：%w", err)
	}
	c.legacyModelFormat = true
	c.Model = make([]apiImpl.Model, 0, len(legacy))
	for _, name := range legacy {
		name = strings.TrimSpace(name)
		if name != "" {
			c.Model = append(c.Model, apiImpl.Model{ID: name, Name: name})
		}
	}
	return nil
}

func LoadConfig() error {
	data, err := Platform.FS.ReadFile(Platform.Paths.ConfigFile)
	if err != nil {
		return err
	}
	Platform.Configs = &Configs{}
	if err := json.Unmarshal(data, Platform.Configs); err != nil {
		return err
	}
	resolveEnabled()

	// 旧字符串格式转换为结构体后立即保存，以持久化新配置格式。
	needsSave := false
	if Platform.Configs.ALLConfig != nil {
		for i := range *Platform.Configs.ALLConfig {
			entry := &(*Platform.Configs.ALLConfig)[i]
			needsSave = needsSave || entry.legacyModelFormat
			if entry.ID == "" {
				entry.ID = newID()
				needsSave = true
			}
		}
	}
	if needsSave {
		if err := SaveConfig(); err != nil {
			return fmt.Errorf("保存迁移后的模型配置失败：%w", err)
		}
	}

	// 启用项没记住模型时默认选第一个，开箱即用；仅在内存中生效，随下次保存落盘。
	if cfg := Platform.Configs.EnablesConfig; cfg != nil && cfg.EnabledModel == "" && len(cfg.Model) > 0 {
		cfg.EnabledModel = cfg.Model[0].ID
	}
	return nil
}

// resolveEnabled 把文件里的 EnablesConfig（一份拷贝）指回配置数组里的对应项，
// 后续选模型、写回时才不会和配置列表脱节；找不到对应项则视为过期数据，一并清空。
func resolveEnabled() {
	cfg := Platform.Configs
	if cfg.EnablesConfig == nil {
		return
	}
	if cfg.ALLConfig != nil {
		for i := range *cfg.ALLConfig {
			if sameConfig(&(*cfg.ALLConfig)[i], cfg.EnablesConfig) {
				cfg.EnablesConfig = &(*cfg.ALLConfig)[i]
				return
			}
		}
	}
	cfg.EnablesConfig = nil
}

// sameConfig 名称不参与比较：改名不影响启用项的归位。
func sameConfig(a, b *Config) bool {
	if a.ID != "" && b.ID != "" {
		return a.ID == b.ID
	}
	if a.ServerAddress != b.ServerAddress || a.Protocol != b.Protocol || a.APIKey != b.APIKey || len(a.Model) != len(b.Model) {
		return false
	}
	for i := range a.Model {
		if a.Model[i].ID != b.Model[i].ID {
			return false
		}
	}
	return true
}

// SaveConfig 把当前配置（含启用项）写回配置文件，先写临时文件再替换，避免写一半损坏原文件。
func SaveConfig() error {
	data, err := json.MarshalIndent(Platform.Configs, "", "    ")
	if err != nil {
		return err
	}
	tmp := Platform.Paths.ConfigFile + ".tmp"
	if err := Platform.FS.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return Platform.FS.MoveFile(tmp, Platform.Paths.ConfigFile)
}
