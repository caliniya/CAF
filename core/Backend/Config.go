package Backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	netclient "github.com/caliniya/caf/NetClient"
	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	"github.com/caliniya/caf/core/credentials"
	"github.com/caliniya/caf/utils/id"
)

type Configs struct {
	SharedModelNotice    string                  `json:"sharedModelNotice,omitempty"`
	SharedModels         bool                    `json:"sharedModels,omitempty"`
	PreferredSharedModel *credentials.ModelGrant `json:"preferredSharedModel,omitempty"`
	ALLConfig            *[]Config               `json:"config"`
	EnablesConfig        *Config
	ToolNormalConfig     *agenttool.ChatToolConfig `json:"toolNormalConfig,omitempty"`
	DefaultMemoryIDs     []string                  `json:"defaultMemoryIds,omitempty"`
	DefaultCompaction    *CompactionDefaults       `json:"defaultCompaction,omitempty"`
}

type Config struct {
	ID                string `json:"ID"`
	Name              string `json:"Name"`
	ServerAddress     string `json:"ServerAddress"`
	Protocol          string `json:"Protocol"`
	HasAPIKey         bool   `json:"HasAPIKey,omitempty"`
	legacyKey         string
	Model             []apiImpl.Model `json:"Model"`
	EnabledModel      string          `json:"EnabledModel"`
	legacyModelFormat bool            `json:"-"`
}

func (c *Config) snapshot() *Config {
	copy := *c
	copy.Model = slices.Clone(c.Model)
	for i := range copy.Model {
		m := &copy.Model[i]
		m.InputModalities = slices.Clone(m.InputModalities)
		m.OutputModalities = slices.Clone(m.OutputModalities)
		m.SupportedLevels = slices.Clone(m.SupportedLevels)
	}
	return &copy
}

// The bridge serializes after releasing configMu. Keep its view independent of later edits.
func (cfg *Configs) snapshot() *Configs {
	view := *cfg
	view.PreferredSharedModel = nil // storage-only; the client already receives the effective selection
	if cfg.ALLConfig != nil {
		all := slices.Clone(*cfg.ALLConfig)
		for i := range all {
			all[i] = *all[i].snapshot()
		}
		view.ALLConfig = &all
	}
	view.resolveEnabled()
	view.ToolNormalConfig = cfg.ToolNormalConfig.Clone()
	view.DefaultMemoryIDs = slices.Clone(cfg.DefaultMemoryIDs)
	if cfg.DefaultCompaction != nil {
		d := *cfg.DefaultCompaction
		view.DefaultCompaction = &d
	}
	return &view
}

func (c *Config) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID            string          `json:"ID"`
		Name          string          `json:"Name"`
		ServerAddress string          `json:"ServerAddress"`
		Protocol      string          `json:"Protocol"`
		APIKey        string          `json:"APIKey"`
		HasAPIKey     bool            `json:"HasAPIKey"`
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
	c.legacyKey, c.HasAPIKey = raw.APIKey, raw.HasAPIKey || raw.APIKey != ""
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

func (c *Core) loadConfig() error {
	if err := c.loadPersonalConfig(); err != nil {
		return err
	}
	c.Configs.SharedModels = false
	c.Configs.SharedModelNotice = ""
	if c.internal != nil {
		c.internal.overlay(c.Configs)
	}
	return nil
}
func (c *Core) loadPersonalConfig() error {
	data, err := c.FS.ReadFile(c.Paths.ConfigFile)
	if errors.Is(err, fs.ErrNotExist) {
		all := []Config{}
		previous := c.Configs
		c.Configs = &Configs{ALLConfig: &all}
		if err := c.writeConfig(c.Configs); err != nil {
			c.Configs = previous
			return fmt.Errorf("创建初始配置失败：%w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	configs := &Configs{}
	if err := json.Unmarshal(data, configs); err != nil {
		return err
	}
	savedEnabled := configs.EnablesConfig
	configs.resolveEnabled()
	if active := configs.EnablesConfig; active != nil && savedEnabled.legacyKey != "" {
		if active.legacyKey != "" && active.legacyKey != savedEnabled.legacyKey {
			return errors.New("启用的配置和配置列表中的密钥不同，原文件已保留")
		}
		// The enabled copy can be the only place an old config kept its key.
		// Matching a stable ID does not authorize moving that key to another origin.
		if active.legacyKey == "" {
			source, err := netclient.ServerOrigin(savedEnabled.ServerAddress)
			target, targetErr := netclient.ServerOrigin(active.ServerAddress)
			if err != nil || targetErr != nil || source != target {
				return errors.New("启用的配置和配置列表中的服务地址不一致，无法迁移密钥，原文件已保留")
			}
		}
		active.legacyKey = savedEnabled.legacyKey
		active.HasAPIKey = true
	}

	// 旧字符串格式转换为结构体后立即保存，以持久化新配置格式。
	needsSave := savedEnabled != nil && savedEnabled.legacyKey != ""
	changes := map[string]credentials.Entry{}
	ids := map[string]bool{}
	if configs.ALLConfig != nil {
		for i := range *configs.ALLConfig {
			entry := &(*configs.ALLConfig)[i]
			needsSave = needsSave || entry.legacyModelFormat
			if entry.ID == "" {
				entry.ID, err = id.New(id.Config)
				if err != nil {
					return err
				}
				needsSave = true
			}
			if ids[entry.ID] {
				return errors.New("模型配置包含重复 ID，原文件已保留")
			}
			ids[entry.ID] = true
			if entry.legacyKey != "" {
				v, e := modelSecret(entry.ServerAddress, entry.legacyKey)
				if e != nil {
					return e
				}
				changes[modelRef(entry.ID)] = v
				entry.legacyKey = ""
				needsSave = true
			}
		}
	}
	previous := c.Configs
	c.Configs = configs
	if needsSave {
		if err := c.withCredentials(changes, true, func() error { return c.writeConfig(configs) }); err != nil {
			c.Configs = previous
			return fmt.Errorf("保存迁移后的模型配置失败：%w", err)
		}
	}

	// 启用项没记住模型时默认选第一个，开箱即用；仅在内存中生效，随下次保存落盘。
	if cfg := configs.EnablesConfig; cfg != nil && cfg.EnabledModel == "" && len(cfg.Model) > 0 {
		cfg.EnabledModel = cfg.Model[0].ID
	}
	return nil
}

// resolveEnabled 把文件里的 EnablesConfig（一份拷贝）指回配置数组里的对应项，
// 后续选模型、写回时才不会和配置列表脱节；找不到对应项则视为过期数据，一并清空。
func (cfg *Configs) resolveEnabled() {
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
	if a.ServerAddress != b.ServerAddress || a.Protocol != b.Protocol || (a.legacyKey != "" && b.legacyKey != "" && a.legacyKey != b.legacyKey) || len(a.Model) != len(b.Model) {
		return false
	}
	for i := range a.Model {
		if a.Model[i].ID != b.Model[i].ID {
			return false
		}
	}
	return true
}

// saveConfig 创建所需目录，先写临时文件再替换配置，避免写一半损坏原文件。
func (c *Core) saveConfig() error {
	if c == nil {
		return errors.New("配置存储尚未初始化")
	}
	if c.internal != nil && c.Configs != nil {
		return c.writeConfig(c.internal.personal(c.Configs))
	}
	return c.writeConfig(c.Configs)
}
func (c *Core) writeConfig(value *Configs) error {
	if c == nil || c.Paths == nil || c.Paths.ConfigFile == "" || c.FS == nil {
		return errors.New("配置存储尚未初始化")
	}
	if value == nil {
		return errors.New("配置尚未加载")
	}
	data, err := json.MarshalIndent(value, "", "    ")
	if err != nil {
		return err
	}
	if err := c.FS.CreateDirectory(filepath.Dir(c.Paths.ConfigFile), 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	tmp := c.Paths.ConfigFile + ".tmp"
	if err := c.FS.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return c.FS.MoveFile(tmp, c.Paths.ConfigFile)
}
