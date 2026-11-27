package Backend

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	platform "github.com/caliniya/caf/platform"
)

func insidePath(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && filepath.IsLocal(rel)
}

// Export always reauthenticates, even on a device configured for automatic login.
// The native picker is the only destination input exposed to the webview.
func (a *Accounts) ExportUserData(ticket, pass string, includeCredentials bool) (string, error) {
	p := a.p
	p.op.Lock()
	defer p.op.Unlock()
	s, err := p.session(ticket, false)
	if err != nil {
		return "", err
	}
	if !s.core.vault.Matches(pass) {
		return "", errors.New("当前密码不正确")
	}
	dest, err := p.fs.SelectDirectory("")
	if err != nil || dest == "" {
		return "", err
	}
	dest, err = platform.PhysicalPath(dest)
	if err != nil {
		return "", fmt.Errorf("无法解析导出目录：%w", err)
	}
	base, err := platform.PhysicalPath(p.base.Data)
	if err != nil {
		return "", fmt.Errorf("无法解析用户目录：%w", err)
	}
	if !filepath.IsAbs(dest) || insidePath(base, dest) {
		return "", errors.New("请在 CAF 数据目录之外选择导出位置")
	}
	f, err := os.CreateTemp(dest, ".caf-export-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	err = s.core.exportUserFiles(z, includeCredentials)
	err = errors.Join(err, z.Close(), f.Close())
	if err != nil {
		return "", err
	}
	target := filepath.Join(dest, "CAF-"+time.Now().Format("20060102-150405")+"-"+rand.Text()[:8]+".zip")
	if err = os.Rename(f.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}
func (c *Core) exportUserFiles(z *zip.Writer, include bool) error {
	root, err := os.OpenRoot(c.Paths.Data)
	if err != nil {
		return err
	}
	defer root.Close()
	// Explicit roots keep other users, the vault, native keys and external workspaces out of the archive.
	for _, tree := range []string{"config", "workspaces.json", "chats", "memory", "qq", "ext", "InternalWorkspaces", "logs"} {
		if _, err = root.Lstat(filepath.FromSlash(tree)); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		err = fs.WalkDir(root.FS(), tree, func(name string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if strings.Contains(name, `\`) {
				return fmt.Errorf("文件名不能安全地跨平台导出：%s", name)
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if tree == "config" && name != "config" && name != "config/MainConfig.json" {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if tree != "InternalWorkspaces" && (strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".migrating")) {
				return nil
			}
			if d.IsDir() {
				_, e := z.Create(name + "/")
				return e
			}
			if !d.Type().IsRegular() {
				return nil
			}
			file := filepath.FromSlash(name)
			out, e := z.Create(name)
			if e != nil {
				return e
			}
			scope := conversationScope(c.Paths.Data, filepath.Join(c.Paths.Data, file))
			if scope != "" {
				b, e := root.ReadFile(file)
				if e != nil {
					return e
				}
				b, e = c.vault.ChatBytes(scope, b, false)
				if e != nil {
					return e
				}
				defer clear(b)
				_, e = out.Write(b)
				return e
			}
			if name == "config/MainConfig.json" || name == "qq/adapters.json" {
				b, e := c.exportConfig(root, file, name, include)
				if e != nil {
					return e
				}
				defer clear(b)
				_, e = out.Write(b)
				return e
			}
			f, e := root.Open(file)
			if e != nil {
				return e
			}
			defer f.Close()
			_, e = io.Copy(out, f)
			return e
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func (c *Core) exportConfig(root *os.Root, file, name string, include bool) ([]byte, error) {
	model := name == "config/MainConfig.json"
	if model {
		include = include && c.internal == nil
		c.configMu.RLock()
		defer c.configMu.RUnlock()
	} else {
		q, e := c.Servers.Get[*QQServer](QQServerName)
		if e != nil {
			return nil, e
		}
		q.mu.Lock()
		defer q.mu.Unlock()
	}
	b, err := root.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err = dec.Decode(&doc); err != nil {
		return nil, err
	}
	put := func(raw any) error {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		if model {
			delete(item, "APIKey")
			has, _ := item["HasAPIKey"].(bool)
			if !include {
				delete(item, "HasAPIKey")
				return nil
			}
			if has {
				id, _ := item["ID"].(string)
				addr, _ := item["ServerAddress"].(string)
				key, e := c.providerKey(&Config{ID: id, HasAPIKey: true}, addr)
				if e != nil {
					return e
				}
				item["APIKey"] = key
			}
		} else {
			delete(item, "token")
			if include {
				id, _ := item["id"].(string)
				entry, e := c.credential(qqRef(id))
				if e != nil {
					return e
				}
				item["token"] = entry.Value
			}
		}
		return nil
	}
	group := "adapters"
	if model {
		group = "config"
		if err := put(doc["EnablesConfig"]); err != nil {
			return nil, err
		}
	}
	list, _ := doc[group].([]any)
	for _, row := range list {
		if err := put(row); err != nil {
			return nil, err
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}
