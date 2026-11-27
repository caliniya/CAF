package Backend

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	memory "github.com/caliniya/caf/Memory"
	"github.com/caliniya/caf/NetClient/NetAPI"
	"github.com/caliniya/caf/core/credentials"
	platform "github.com/caliniya/caf/platform"
	"github.com/caliniya/caf/utils/id"
)

// Profiles owns the desktop session lifecycle. Only Accounts is bound to the webview.
// A fresh Core (including its services, caches and paths) is allocated for every login.
type Profiles struct {
	op                       sync.Mutex // serialize account changes, including KDF work
	mu                       sync.Mutex // publish/revoke tickets independently of ongoing chat calls
	base                     platform.Paths
	fs                       platform.FilePlatform
	device                   credentials.DeviceKeys
	net                      *NetAPI.NetAPIClient
	ctx                      context.Context
	active                   *profileSession
	triedDefault             bool
	modelKey                 []byte // model sharing only; remains available when switching users in this process
	slot, defaultID, warning string
}
type profileSession struct {
	core   *Core
	ticket string
	ready  bool
	ctx    context.Context
	cancel context.CancelFunc
	calls  sync.WaitGroup
}
type Accounts struct{ p *Profiles }
type profileFile struct {
	dir   string
	vault *credentials.Store
}

func NewProfiles(base platform.Paths, files platform.FilePlatform, device credentials.DeviceKeys, net *NetAPI.NetAPIClient) *Profiles {
	return &Profiles{base: base, fs: files, device: device, net: net, ctx: context.Background()}
}
func (p *Profiles) API() *Accounts            { return &Accounts{p: p} }
func (p *Profiles) Start(ctx context.Context) { p.op.Lock(); defer p.op.Unlock(); p.ctx = ctx }

// The original user stays at the data root so existing internal workspace paths stay valid.
func (p *Profiles) profiles() ([]profileFile, error) {
	if !filepath.IsAbs(p.base.Data) || p.fs == nil {
		return nil, errors.New("用户数据目录无效")
	}
	p.slot = ""
	var out []profileFile
	var issues []error
	seen := map[string]bool{}
	add := func(dir string) {
		v, err := credentials.OpenLocked(p.fs, filepath.Join(dir, "secrets.vault"), p.device)
		if err != nil {
			issues = append(issues, fmt.Errorf("读取用户密钥库 %s：%w", dir, err))
			return
		}
		if !v.Status().Configured {
			return
		}
		key := v.ID()
		if seen[key] {
			issues = append(issues, errors.New("数据目录中存在重复的用户密钥库"))
			return
		}
		seen[key] = true
		out = append(out, profileFile{dir, v})
		if dir == p.base.Data {
			p.slot = key
		}
	}
	add(p.base.Data)
	dir := filepath.Join(p.base.Data, "profiles")
	info, err := p.fs.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, errors.Join(issues...)
	}
	if err != nil {
		return out, errors.Join(append(issues, err)...)
	}
	if !info.IsDir() {
		return out, errors.Join(append(issues, errors.New("用户目录无效"))...)
	}
	entries, err := p.fs.ReadDir(dir)
	if err != nil {
		return out, errors.Join(append(issues, err)...)
	}
	for _, entry := range entries {
		if entry.IsDir() && id.Valid(entry.Name(), id.User) {
			add(filepath.Join(dir, entry.Name()))
		}
	}
	if len(out) > 0 && p.slot == "" {
		issues = append(issues, errors.New("首个用户的密钥库缺失或损坏"))
	}
	return out, errors.Join(issues...)
}
func (p *Profiles) selectProfile(f profileFile) {
	paths := p.base
	paths.Data = f.dir
	paths.ConfigFile = filepath.Join(f.dir, "config", "MainConfig.json")
	paths.InternalWorkspaces = filepath.Join(f.dir, "InternalWorkspaces")
	paths.Logs = filepath.Join(f.dir, "logs")
	c := &Core{Paths: &paths, FS: p.fs, APIClient: p.net, DeviceKeys: p.device, vault: f.vault, profileRoot: p.base.Data}
	if f.dir != p.base.Data {
		c.internal = &internalModels{}
		if err := p.loadSharedModels(c); err != nil {
			c.internal.notice = err.Error()
			p.warning = err.Error()
		}
	} else if f.vault.HasSharedModels() {
		p.provisionSharing(f.vault)
	}
	c.InitServers()
	ctx, cancel := context.WithCancel(p.ctx)
	s := &profileSession{core: c, ticket: rand.Text(), ctx: ctx, cancel: cancel}
	p.mu.Lock()
	p.active = s
	p.mu.Unlock()
}
func (p *Profiles) status(configured bool) credentials.Status {
	st := credentials.Status{Configured: configured, Warning: p.warning}
	if s := p.active; s != nil {
		st.Unlocked, st.Remembered = true, p.defaultID == s.core.vault.ID()
		st.Role = "owner"
		if s.core.internal != nil {
			st.Role = "internal"
		}
	}
	return st
}
func (a *Accounts) GetVaultStatus() (credentials.Status, error) {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	if s := p.active; s != nil {
		return p.status(true), nil
	}
	files, err := p.profiles()
	if err != nil {
		if len(files) == 0 {
			return credentials.Status{}, err
		}
		p.warning = "部分用户密钥库无法读取，其他用户仍可登录。"
	}
	if !p.triedDefault && len(files) > 0 {
		p.triedDefault = true
		if p.device != nil && p.slot != "" {
			record, e := p.device.Load(p.slot)
			defer clear(record)
			if e != nil {
				p.warning = "系统凭据库不可用，请输入密码登录。"
			} else if id, suspended := credentials.DeviceDefault(record, p.slot); suspended {
				p.defaultID = id
			} else if len(record) > 0 {
				p.warning = "本机默认登录记录不可用，请输入密码。"
				for _, f := range files {
					if match, e := f.vault.UnlockDefault(record, f.dir == p.base.Data); match {
						if e == nil {
							p.defaultID = f.vault.ID()
							p.warning = ""
							p.selectProfile(f)
						}
						break
					}
				}
			}
		}
	}
	return p.status(len(files) > 0), nil
}
func (a *Accounts) CreateVault(pass string) error {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	if p.active != nil {
		return errors.New("请先退出当前用户")
	}
	files, err := p.profiles()
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return errors.New("主用户已存在，请登录；只有主用户可以在跨平台设置中创建内部用户")
	}
	dir := p.base.Data
	v, err := credentials.OpenLocked(p.fs, filepath.Join(dir, "secrets.vault"), p.device)
	if err != nil {
		return err
	}
	if err = v.Create(pass); err != nil {
		return err
	}
	p.slot = v.ID()
	p.triedDefault = true
	p.warning = ""
	p.selectProfile(profileFile{dir, v})
	return nil
}
func (a *Accounts) UnlockVault(pass string) error {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	if p.active != nil {
		return errors.New("请先退出当前用户")
	}
	files, err := p.profiles()
	if err != nil && len(files) == 0 {
		return err
	}
	for _, f := range files {
		if err := f.vault.Unlock(pass); err == nil {
			p.triedDefault = true
			p.warning = ""
			// Resume a suspended default only after its own successful manual login.
			if p.device != nil && p.slot != "" {
				record, readErr := p.device.Load(p.slot)
				known, suspended := credentials.DeviceDefault(record, p.slot)
				if readErr == nil {
					p.defaultID = known
				}
				if suspended && known == f.vault.ID() {
					if err := f.vault.SaveDefault(p.slot); err != nil {
						p.warning = "已登录，未能恢复本机自动登录。"
					}
				}
				clear(record)
			}
			p.selectProfile(f)
			return nil
		}
	}
	return errors.New("密码不正确，或对应用户的密钥库已损坏")
}
func (a *Accounts) StartVaultSession() (string, error) {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	s := p.active
	if s == nil {
		return "", credentials.ErrLocked
	}
	if s.ready {
		return s.ticket, nil
	}
	c := s.core
	if err := c.StartVaultSession(); err != nil {
		return "", err
	}
	if err := c.protectConversations(); err != nil {
		return "", err
	}
	if _, err := c.GetChatMemorySettings(""); err != nil {
		return "", err
	}
	chat, _ := c.Servers.Get[*ChatServer](ChatServerName)
	approve, _ := c.Servers.Get[*ToolApprovalServer](ToolApprovalServerName)
	qq, _ := c.Servers.Get[*QQServer](QQServerName)
	if err := chat.Start(s.ctx); err != nil {
		return "", err
	}
	if err := approve.Start(s.ctx); err != nil {
		return "", err
	}
	if err := qq.Start(s.ctx); err != nil {
		p.warning = fmt.Sprintf("部分 QQ 扩展未能启动：%v", err)
	}
	p.mu.Lock()
	s.ready = true
	p.mu.Unlock()
	return s.ticket, nil
}
func (p *Profiles) session(ticket string, allowPending bool) (*profileSession, error) {
	s := p.active
	if s == nil || !(s.ready && ticket == s.ticket || allowPending && !s.ready && ticket == "") {
		return nil, credentials.ErrLocked
	}
	return s, nil
}
func (a *Accounts) SetDefaultProfile(ticket string, on bool) error {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	s, err := p.session(ticket, true)
	if err != nil {
		return err
	}
	v := s.core.vault
	if p.slot == "" {
		return errors.New("首个用户密钥库不可用，暂不能设置本设备的默认用户")
	}
	if on {
		if err = v.SaveDefault(p.slot); err != nil {
			return err
		}
		p.defaultID = v.ID()
	} else if p.defaultID == v.ID() {
		if p.device == nil {
			return errors.New("系统凭据库不可用")
		}
		if err = p.device.Forget(p.slot); err != nil {
			return err
		}
		p.defaultID = ""
	}
	p.warning = ""
	return nil
}
func (a *Accounts) ChangePassword(ticket, old, next string) error {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	s, err := p.session(ticket, false)
	if err != nil {
		return err
	}
	if err := credentials.ValidatePassword(next); err != nil {
		return err
	}
	if !s.core.vault.Matches(old) {
		return errors.New("当前密码不正确")
	}
	if old == next {
		return errors.New("新密码与当前密码相同")
	}
	files, err := p.profiles()
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.vault.ID() != s.core.vault.ID() && f.vault.Matches(next) {
			return errors.New("此密码已被使用，请换一个密码")
		}
	}
	return s.core.vault.ChangePassword(old, next)
}
func (a *Accounts) Logout(ticket string) error {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	s, err := p.session(ticket, true)
	if err != nil {
		return err
	}
	if p.defaultID == s.core.vault.ID() {
		if err := s.core.vault.SuspendDefault(p.slot); err != nil {
			return fmt.Errorf("无法暂停自动登录，请先关闭本机默认登录再退出：%w", err)
		}
	}
	if err := p.closeLocked(context.Background()); err != nil {
		p.warning = "已退出，停止服务时遇到问题：" + err.Error()
	}
	return nil
}
func (p *Profiles) Close(ctx context.Context) error {
	p.op.Lock()
	defer p.op.Unlock()
	defer func() { clear(p.modelKey); p.modelKey = nil }()
	return p.closeLocked(ctx)
}
func (p *Profiles) closeLocked(ctx context.Context) error {
	p.mu.Lock()
	s := p.active
	if s != nil {
		s.ready = false
		p.active = nil
	}
	p.mu.Unlock()
	if s == nil {
		return nil
	}
	p.triedDefault = true
	s.cancel()
	c := s.core
	q, _ := c.Servers.Get[*QQServer](QQServerName)
	ch, _ := c.Servers.Get[*ChatServer](ChatServerName)
	ap, _ := c.Servers.Get[*ToolApprovalServer](ToolApprovalServerName)
	err := errors.Join(q.Close(ctx), ch.Close(ctx), ap.Close(ctx))
	s.calls.Wait()
	// A previously admitted request might have started a listener while Close was draining.
	err = errors.Join(err, q.Close(ctx), ch.Close(ctx))
	if m, e := c.Servers.Get[*memory.MemoryServer](memory.MemoryServerName); e == nil {
		err = errors.Join(err, m.Close(ctx))
	}
	c.vault.Lock()
	return err
}
