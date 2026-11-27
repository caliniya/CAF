package Backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	netclient "github.com/caliniya/caf/NetClient"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	"github.com/caliniya/caf/core/credentials"
	"github.com/caliniya/caf/utils/id"
)

type InternalUser struct {
	ID     string                   `json:"id"`
	Models []credentials.ModelGrant `json:"models"`
}

// The app runs one session per data directory. Only the owner can change the catalogue
// or grants. Switching users drains existing calls and constructs a new set of routes.
type internalModels struct {
	notice            string
	available         []Config
	routes            map[credentials.ModelGrant]netclient.Options
	personalProviders *[]Config
	personalDefault   *Config
}

func (p *Profiles) provisionSharing(owner *credentials.Store) {
	key, err := owner.ModelSharingKey()
	if err != nil {
		p.warning = err.Error()
		return
	}
	clear(p.modelKey)
	p.modelKey = key
	slot := "models-v1/" + owner.ID()
	if p.device != nil {
		saved, _ := p.device.Load(slot)
		defer clear(saved)
		if bytes.Equal(saved, key) || p.device.Save(slot, key) == nil {
			return
		}
	}
	p.warning = "系统凭据库不可用。当前仍可共享模型，重启 CAF 后请主用户先登录一次。"
}
func (p *Profiles) loadSharedModels(c *Core) error {
	owner, err := credentials.OpenLocked(p.fs, filepath.Join(p.base.Data, "secrets.vault"), nil)
	if err != nil {
		return fmt.Errorf("无法读取主用户的共享授权：%w", err)
	}
	if !owner.HasSharedModels() {
		return nil
	}
	key := p.modelKey
	if len(key) == 0 && p.device != nil {
		key, _ = p.device.Load("models-v1/" + owner.ID())
		defer clear(key)
	}
	grants, err := owner.SharedModels(key, c.vault.ID())
	if err != nil || len(grants) == 0 {
		return err
	}
	b, err := p.fs.ReadFile(filepath.Join(p.base.Data, "config", "MainConfig.json"))
	if err != nil {
		return err
	}
	var catalog Configs
	if err = json.Unmarshal(b, &catalog); err != nil {
		return err
	}
	if catalog.ALLConfig == nil {
		return nil
	}
	c.internal.routes = map[credentials.ModelGrant]netclient.Options{}
	for _, provider := range *catalog.ALLConfig {
		view := Config{ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol}
		for _, model := range provider.Model {
			g := credentials.ModelGrant{ConfigID: provider.ID, ModelID: model.ID}
			entry, ok := grants[g]
			if !ok {
				continue
			}
			opts := netclient.Options{ServerAddress: provider.ServerAddress, Model: model.ID}
			if provider.HasAPIKey {
				origin, err := netclient.ServerOrigin(provider.ServerAddress)
				if err != nil || entry.Value == "" || origin != entry.Origin {
					continue
				}
				opts.APIKey = entry.Value
			}
			c.internal.routes[g] = opts
			view.Model = append(view.Model, model)
		}
		if len(view.Model) > 0 {
			view.EnabledModel = view.Model[0].ID
			c.internal.available = append(c.internal.available, view)
		}
	}
	return nil
}

// Keep old local configurations on disk, but never use them as authorization. Only a
// preferred pair and the internal user's own chat defaults are written back.
func (m *internalModels) overlay(cfg *Configs) {
	m.personalProviders, m.personalDefault = cfg.ALLConfig, cfg.EnablesConfig
	cfg.SharedModelNotice = m.notice
	all := make([]Config, len(m.available))
	copy(all, m.available)
	cfg.ALLConfig, cfg.EnablesConfig, cfg.SharedModels = &all, nil, true
	if len(all) > 0 {
		cfg.EnablesConfig = &all[0]
	}
	if pref := cfg.PreferredSharedModel; pref != nil {
		if _, ok := m.routes[*pref]; ok {
			for i := range all {
				if all[i].ID == pref.ConfigID {
					all[i].EnabledModel = pref.ModelID
					cfg.EnablesConfig = &all[i]
					break
				}
			}
		}
	}
}
func (m *internalModels) personal(cfg *Configs) *Configs {
	v := *cfg
	v.SharedModelNotice = ""
	v.ALLConfig, v.EnablesConfig, v.SharedModels = m.personalProviders, m.personalDefault, false
	if sel := cfg.EnablesConfig; sel != nil {
		v.PreferredSharedModel = &credentials.ModelGrant{ConfigID: sel.ID, ModelID: sel.EnabledModel}
	}
	return &v
}

func (p *Profiles) owner(ticket string) (*Core, error) {
	s, err := p.session(ticket, false)
	if err != nil {
		return nil, err
	}
	if s.core.internal != nil {
		return nil, errors.New("只有主用户可以管理内部用户和模型授权")
	}
	return s.core, nil
}
func availablePair(configs *Configs, g credentials.ModelGrant) bool {
	if configs == nil || configs.ALLConfig == nil {
		return false
	}
	for _, cfg := range *configs.ALLConfig {
		if cfg.ID == g.ConfigID {
			return slices.ContainsFunc(cfg.Model, func(m api.Model) bool { return m.ID == g.ModelID })
		}
	}
	return false
}
func (a *Accounts) ListInternalUsers(ticket string) ([]InternalUser, error) {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	c, err := p.owner(ticket)
	if err != nil {
		return nil, err
	}
	files, err := p.profiles()
	if err != nil {
		return nil, err
	}
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	out := []InternalUser{}
	for _, f := range files {
		if f.dir == p.base.Data {
			continue
		}
		uid := f.vault.ID()
		entry, e := c.vault.Get(credentials.GrantPrefix + uid)
		var saved []credentials.ModelGrant
		if e == nil {
			if err := json.Unmarshal([]byte(entry.Value), &saved); err != nil {
				return nil, err
			}
		} else if !errors.Is(e, credentials.ErrNoEntry) {
			return nil, e
		}
		row := InternalUser{ID: uid, Models: []credentials.ModelGrant{}}
		for _, g := range saved {
			if availablePair(c.Configs, g) {
				row.Models = append(row.Models, g)
			}
		}
		out = append(out, row)
	}
	return out, nil
}
func (p *Profiles) saveGrants(c *Core, uid string, models []credentials.ModelGrant, commit func() error) (InternalUser, error) {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	row := InternalUser{ID: uid, Models: []credentials.ModelGrant{}}
	seen := map[credentials.ModelGrant]bool{}
	for _, g := range models {
		if seen[g] {
			continue
		}
		if !availablePair(c.Configs, g) {
			return InternalUser{}, errors.New("模型不在主用户的供应商配置中，请重新选择")
		}
		seen[g] = true
		row.Models = append(row.Models, g)
	}
	var entry credentials.Entry
	if len(row.Models) > 0 {
		b, err := json.Marshal(row.Models)
		if err != nil {
			return InternalUser{}, err
		}
		entry.Value = string(b)
	}
	hadSharing := c.vault.HasSharedModels()
	if err := c.vault.Apply(map[string]credentials.Entry{credentials.GrantPrefix + uid: entry}, false, commit); err != nil {
		return InternalUser{}, err
	}
	if len(row.Models) > 0 {
		p.provisionSharing(c.vault)
	} else if hadSharing && !c.vault.HasSharedModels() {
		clear(p.modelKey)
		p.modelKey = nil
		if p.device != nil {
			_ = p.device.Forget("models-v1/" + c.vault.ID())
		}
	}
	return row, nil
}
func (a *Accounts) SetInternalUserModels(ticket, uid string, models []credentials.ModelGrant) (InternalUser, error) {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	c, err := p.owner(ticket)
	if err != nil {
		return InternalUser{}, err
	}
	files, err := p.profiles()
	if err != nil {
		return InternalUser{}, err
	}
	for _, f := range files {
		if f.dir != p.base.Data && f.vault.ID() == uid {
			return p.saveGrants(c, uid, models, nil)
		}
	}
	return InternalUser{}, errors.New("内部用户不存在")
}
func (a *Accounts) CreateInternalUser(ticket, pass string, models []credentials.ModelGrant) (InternalUser, error) {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	c, err := p.owner(ticket)
	if err != nil {
		return InternalUser{}, err
	}
	if err = credentials.ValidatePassword(pass); err != nil {
		return InternalUser{}, err
	}
	files, err := p.profiles()
	if err != nil {
		return InternalUser{}, err
	}
	for _, f := range files {
		if f.vault.Matches(pass) {
			return InternalUser{}, errors.New("此密码已被使用，请换一个密码")
		}
	}
	folder, err := id.New(id.User)
	if err != nil {
		return InternalUser{}, err
	}
	parent := filepath.Join(p.base.Data, "profiles")
	temp := ".creating-" + folder
	stage := filepath.Join(parent, temp)
	if err = p.fs.CreateDirectory(stage, 0700); err != nil {
		return InternalUser{}, err
	}
	defer p.fs.RemoveAllInRoot(parent, temp)
	v, err := credentials.OpenLocked(p.fs, filepath.Join(stage, "secrets.vault"), nil)
	if err != nil {
		return InternalUser{}, err
	}
	if err = v.Create(pass); err != nil {
		return InternalUser{}, err
	}
	defer v.Lock()
	return p.saveGrants(c, v.ID(), models, func() error { return p.fs.MoveFile(stage, filepath.Join(parent, folder)) })
}
