package Backend

import (
	"errors"
	"path/filepath"

	netclient "github.com/caliniya/caf/NetClient"
	"github.com/caliniya/caf/core/credentials"
)

// Only these operations are bound to Wails; there is deliberately no credential getter.
func (c *Core) credentialVault() (*credentials.Store, error) {
	c.vaultMu.Lock()
	defer c.vaultMu.Unlock()
	if c.vault != nil {
		return c.vault, nil
	}
	if c.Paths == nil || !filepath.IsAbs(c.Paths.Data) {
		return nil, errors.New("密钥库数据目录未初始化")
	}
	v, err := credentials.Open(c.FS, filepath.Join(c.Paths.Data, "secrets.vault"), c.DeviceKeys)
	if err == nil {
		c.vault = v
	}
	return v, err
}
func (c *Core) GetVaultStatus() (credentials.Status, error) {
	v, e := c.credentialVault()
	if e != nil {
		return credentials.Status{}, e
	}
	return v.Status(), nil
}
func (c *Core) CreateVault(pass string) error {
	v, e := c.credentialVault()
	if e != nil {
		return e
	}
	return v.Create(pass)
}
func (c *Core) UnlockVault(pass string) error {
	v, e := c.credentialVault()
	if e != nil {
		return e
	}
	return v.Unlock(pass)
}
func (c *Core) SetVaultRemembered(remember bool) error {
	v, e := c.credentialVault()
	if e != nil {
		return e
	}
	return v.Remember(remember)
}

// Finish both migrations before exposing the application or resuming background clients.
func (c *Core) StartVaultSession() error {
	v, err := c.credentialVault()
	if err != nil {
		return err
	}
	if !v.Status().Unlocked {
		return credentials.ErrLocked
	}
	if _, err = c.LoadConfigs(); err != nil {
		return err
	}
	if _, err = c.ListQQAdapters(); err != nil {
		return err
	}
	if c.OnVaultReady != nil {
		c.vaultReadyOnce.Do(c.OnVaultReady)
	}
	return nil
}
func (c *Core) withCredentials(changes map[string]credentials.Entry, migration bool, commit func() error) error {
	if len(changes) == 0 {
		return commit()
	}
	v, err := c.credentialVault()
	if err != nil {
		return err
	}
	return v.Apply(changes, migration, commit)
}
func (c *Core) credential(ref string) (credentials.Entry, error) {
	v, e := c.credentialVault()
	if e != nil {
		return credentials.Entry{}, e
	}
	return v.Get(ref)
}
func modelRef(id string) string { return "model/" + id }
func qqRef(id string) string    { return "qq/" + id }
func modelSecret(address, key string) (credentials.Entry, error) {
	origin, e := netclient.ServerOrigin(address)
	return credentials.Entry{Value: key, Origin: origin}, e
}
func (c *Core) providerKey(cfg *Config, address string) (string, error) {
	if !cfg.HasAPIKey {
		return "", nil
	}
	v, e := c.credential(modelRef(cfg.ID))
	if e != nil {
		return "", e
	}
	origin, e := netclient.ServerOrigin(address)
	if e != nil {
		return "", e
	}
	if origin != v.Origin {
		return "", errors.New("服务地址已更换，请为新地址重新填写 API Key")
	}
	return v.Value, nil
}
func (c *Core) providerOptions(cfg *Config, model string) (netclient.Options, error) {
	opts := netclient.Options{ServerAddress: cfg.ServerAddress, Model: model}
	if c.internal != nil {
		var ok bool
		opts, ok = c.internal.routes[credentials.ModelGrant{ConfigID: cfg.ID, ModelID: model}]
		if !ok {
			return netclient.Options{}, errors.New("主用户未授权使用此模型，请重新选择模型")
		}
	} else {
		key, err := c.providerKey(cfg, opts.ServerAddress)
		if err != nil {
			return netclient.Options{}, err
		}
		opts.APIKey = key
	}
	for _, m := range cfg.Model {
		if m.ID == model {
			opts.SearchMode = netclient.ResolveModelCapabilities(opts.ServerAddress, m).NativeSearchMode
			break
		}
	}
	return opts, nil
}

func (c *Core) ProbeSavedModels(id, address string) (*ModelProbeResult, error) {
	cfg, err := c.configByID(id)
	if err != nil {
		return nil, err
	}
	key, err := c.providerKey(cfg, address)
	if err != nil {
		return nil, err
	}
	return c.ProbeModels(address, key)
}
