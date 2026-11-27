package Backend

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/caliniya/caf/core/credentials"
)

// The same QQ file remains readable after archiving its owning client directory.
func conversationScope(base, name string) string {
	rel, err := filepath.Rel(base, name)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	rel = strings.TrimSuffix(filepath.ToSlash(rel), ".tmp")
	if !strings.HasSuffix(rel, ".json") {
		return ""
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 2 && parts[0] == "chats" {
		return rel
	}
	rest, ok := strings.CutPrefix(rel, "ext/clients/")
	if !ok {
		rest, ok = strings.CutPrefix(rel, "ext/archive/clients/")
	}
	if !ok {
		return ""
	}
	parts = strings.Split(rest, "/")
	if len(parts) < 4 || parts[1] != "qq" {
		return ""
	}
	if (len(parts) == 4 && parts[3] == "contexts.json") || (len(parts) == 5 && parts[3] == "sessions") || (len(parts) == 6 && parts[3] == "history") {
		return strings.Join(parts[1:], "/")
	}
	return ""
}
func (c *Core) readConversationFile(path string) ([]byte, error) {
	data, err := c.FS.ReadFile(path)
	if err != nil || c.profileRoot == "" {
		return data, err
	}
	if scope := conversationScope(c.Paths.Data, path); scope != "" {
		return c.vault.ChatBytes(scope, data, false)
	}
	return data, nil
}

// Migrate before publishing the session. Only the encrypted vault records completion, so an
// interrupted pass resumes safely while ordinary reads never downgrade to plaintext.
func (c *Core) protectConversations() error {
	const marker = "format/encrypted-conversations-v1"
	if _, err := c.vault.Get(marker); err == nil {
		return nil
	} else if !errors.Is(err, credentials.ErrNoEntry) {
		return err
	}
	root, err := os.OpenRoot(c.Paths.Data)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, dir := range []string{"chats", "ext"} {
		if _, err := root.Lstat(filepath.FromSlash(dir)); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		err = fs.WalkDir(root.FS(), dir, func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("对话目录含有符号链接：%s", name)
			}
			if d.IsDir() {
				return nil
			}
			scope := conversationScope(c.Paths.Data, filepath.Join(c.Paths.Data, filepath.FromSlash(name)))
			if scope == "" {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("无效的对话文件：%s", name)
			}
			file := filepath.FromSlash(name)
			data, err := root.ReadFile(file)
			if err != nil {
				return err
			}
			defer clear(data)
			if credentials.IsEncryptedChat(data) {
				_, err = c.vault.ChatBytes(scope, data, false)
				return err
			}
			if !strings.HasSuffix(file, ".tmp") && !json.Valid(data) {
				return fmt.Errorf("无法迁移损坏的对话 %s，已保留原文件", name)
			}
			enc, err := c.vault.ChatBytes(scope, data, true)
			if err != nil {
				return err
			}
			tmp := file + "." + rand.Text() + ".migrating"
			f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			defer root.Remove(tmp)
			_, err = f.Write(enc)
			if err = errors.Join(err, f.Close()); err != nil {
				return err
			}
			return root.Rename(tmp, file)
		})
		if err != nil {
			return err
		}
	}
	return c.vault.Apply(map[string]credentials.Entry{marker: {Value: "1"}}, false, nil)
}
