package Backend

import (
	"context"
	"errors"
	"slices"
	"time"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
)

type CompactionDefaults struct {
	chat.CompactionMode
	WindowPercent  int `json:"windowPercent"`
	TokenThreshold int `json:"tokenThreshold"`
}

type ChatDefaults struct {
	Tools            *agenttool.ChatToolConfig `json:"tools"`
	MemoryLibraryIDs []string                  `json:"memoryLibraryIds"`
	Compaction       CompactionDefaults        `json:"compaction"`
}

func builtInCompaction() CompactionDefaults {
	return CompactionDefaults{CompactionMode: chat.CompactionMode{Enabled: true}, WindowPercent: 70, TokenThreshold: 500000}
}

func (c *Core) defaultTools() *agenttool.ChatToolConfig {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	if c.Configs == nil {
		return nil
	}
	return c.Configs.ToolNormalConfig
}

func (c *Core) chatDefaults() ChatDefaults {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	result := ChatDefaults{MemoryLibraryIDs: []string{}, Compaction: builtInCompaction()}
	if cfg := c.Configs; cfg != nil {
		result.Tools = cfg.ToolNormalConfig.Clone()
		result.MemoryLibraryIDs = append(result.MemoryLibraryIDs, cfg.DefaultMemoryIDs...)
		if cfg.DefaultCompaction != nil {
			result.Compaction = *cfg.DefaultCompaction
		}
	}
	return result
}

func (c *Core) validDefaultMemories(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	ctx := context.Background()
	server, err := c.memoryServer(ctx)
	if err != nil {
		return nil, err
	}
	libraries, err := server.ListRecollections(ctx)
	if err != nil {
		return nil, err
	}
	return validMemoryLibraryIDs(ids, libraries), nil
}

func (c *Core) newChatMemoryIDs() ([]string, error) {
	return c.validDefaultMemories(c.chatDefaults().MemoryLibraryIDs)
}

func (c *Core) GetChatDefaults() (ChatDefaults, error) {
	defaults := c.chatDefaults()
	ids, err := c.validDefaultMemories(defaults.MemoryLibraryIDs)
	defaults.MemoryLibraryIDs = ids
	return defaults, err
}

func (c *Core) SetChatDefaults(value ChatDefaults) (ChatDefaults, error) {
	p := &value.Compaction
	if p.WindowPercent < 1 || p.WindowPercent > 100 || p.TokenThreshold < 1 || p.TokenThreshold > 2147483647 {
		return ChatDefaults{}, errors.New("窗口比例须为 1–100 的整数，token 阈值须为 1–2147483647 的整数")
	}
	if !p.Enabled {
		p.Automatic = false
	}
	value.Tools = value.Tools.Clone()
	if tools := value.Tools; tools != nil {
		if tools.Permission < 0 || tools.Permission > agenttool.PermissionFullAccess || len(tools.AllowedTools) > 0 {
			return ChatDefaults{}, errors.New("无效的默认工具权限；单个对话的授权不能作为全局默认")
		}
		for setID, set := range tools.Sets {
			registered := agenttool.ToolSets[setID]
			if registered == nil {
				return ChatDefaults{}, errors.New("未知工具集：" + setID)
			}
			for kit := range set.Kits {
				if registered.Meta().ToolKits[kit] == nil {
					return ChatDefaults{}, errors.New("未知工具模块：" + kit)
				}
			}
		}
		tools.Normalize()
	}
	ids, err := c.validDefaultMemories(value.MemoryLibraryIDs)
	if err != nil {
		return ChatDefaults{}, err
	}
	value.MemoryLibraryIDs = ids
	c.configMu.Lock()
	defer c.configMu.Unlock()
	cfg := c.Configs
	if cfg == nil {
		return ChatDefaults{}, errors.New("全局配置尚未加载")
	}
	oldTools, oldMemory, oldCompaction := cfg.ToolNormalConfig, cfg.DefaultMemoryIDs, cfg.DefaultCompaction
	cfg.ToolNormalConfig, cfg.DefaultMemoryIDs, cfg.DefaultCompaction = value.Tools.Clone(), slices.Clone(ids), &value.Compaction
	if err := c.saveConfig(); err != nil {
		cfg.ToolNormalConfig, cfg.DefaultMemoryIDs, cfg.DefaultCompaction = oldTools, oldMemory, oldCompaction
		return ChatDefaults{}, err
	}
	return value, nil
}

// mode 为 nil 时恢复继承；立即压缩是独立的一次性操作，不更改这里的选择。
func (c *Core) SetChatCompactionMode(id string, mode *chat.CompactionMode) (*chat.ChatTask, error) {
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.sending[id] != nil {
		return nil, errors.New("对话正在生成，暂时无法修改压缩设置")
	}
	dir, err := c.chatDirectory()
	if err != nil {
		return nil, err
	}
	session, err := c.readChatLocked(dir, id)
	if err != nil {
		return nil, err
	}
	if mode != nil {
		copy := *mode
		copy.Automatic = copy.Enabled && copy.Automatic
		mode = &copy
	}
	session.CompactionMode = mode
	session.UpdatedAt = time.Now().UTC()
	if err := c.saveChatLocked(dir, session); err != nil {
		return nil, err
	}
	return session, nil
}
