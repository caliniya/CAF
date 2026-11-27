// Package credentials stores CAF secrets in a portable, passphrase-protected file.
package credentials

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"sync"
	"unicode/utf8"

	platform "github.com/caliniya/caf/platform"
	"golang.org/x/crypto/argon2"
)

var ErrLocked = errors.New("请先解锁密钥库")
var ErrMissing = errors.New("请先创建密钥库")
var ErrNoEntry = errors.New("密钥库中没有此凭据，请重新填写")

// DeviceKeys stores device-local auto-login records and independent model-sharing keys.
// Missing keys return (nil, nil); implementations must not fall back to a local file.
type DeviceKeys interface {
	Load(id string) ([]byte, error)
	Save(id string, key []byte) error
	Forget(id string) error
}

type Status struct {
	Configured bool   `json:"configured"`
	Unlocked   bool   `json:"unlocked"`
	Remembered bool   `json:"remembered"`
	Warning    string `json:"warning,omitempty"`
	Role       string `json:"role,omitempty"`
}
type Entry struct {
	Value  string `json:"value"`
	Origin string `json:"origin,omitempty"`
}
type envelope struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Salt    []byte `json:"salt"`
	Wrapped []byte `json:"wrapped"`
	Payload []byte `json:"payload"`
	Models  []byte `json:"sharedModels,omitempty"`
}

// v1 fixes all algorithm and cost parameters; untrusted files cannot increase KDF cost.
// NewGCMWithRandomNonce prefixes each ciphertext with a fresh 96-bit nonce.
func derive(pass string, salt []byte) []byte {
	return argon2.IDKey([]byte(pass), salt, 3, 64*1024, 2, 32)
}
func gcm(key []byte) (cipher.AEAD, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	return cipher.NewGCMWithRandomNonce(b)
}
func (e envelope) aad(payload bool) []byte {
	ad := append([]byte("CAF/vault/1/wrap/"+e.ID), e.Salt...)
	if payload {
		ad = append([]byte("CAF/vault/1/data/"+e.ID), e.Salt...)
		ad = append(ad, e.Wrapped...)
	}
	return ad
}

const maxFile = 8 << 20

type Store struct {
	mu         sync.Mutex
	fs         platform.FilePlatform
	path       string
	device     DeviceKeys
	env        envelope
	disk       []byte
	key        []byte
	entries    map[string]Entry
	remembered bool
	warning    string
}

// Open never creates or modifies a vault and never prompts for a passphrase.
func Open(f platform.FilePlatform, path string, device DeviceKeys) (*Store, error) {
	return open(f, path, device, true)
}

// OpenLocked reads only the envelope during profile discovery.
func OpenLocked(f platform.FilePlatform, path string, device DeviceKeys) (*Store, error) {
	return open(f, path, device, false)
}

func open(f platform.FilePlatform, path string, device DeviceKeys, auto bool) (*Store, error) {
	if f == nil || !filepath.IsAbs(path) {
		return nil, errors.New("密钥库存储路径无效")
	}
	s := &Store{fs: f, path: path, device: device}
	info, err := f.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFile {
		return nil, errors.New("密钥库文件无效")
	}
	data, err := f.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e envelope
	if json.Unmarshal(data, &e) != nil || e.Version != 1 || len(e.ID) != 32 || len(e.Salt) != 16 || len(e.Wrapped) != 60 || len(e.Payload) < 28 {
		return nil, errors.New("密钥库格式无效或版本不受支持，原文件已保留")
	}
	if _, err = hex.DecodeString(e.ID); err != nil {
		return nil, errors.New("密钥库标识无效")
	}
	s.env, s.disk = e, data
	if auto && device != nil {
		key, err := device.Load(e.ID)
		if err != nil {
			s.warning = "系统凭据库不可用，可使用主密码解锁。"
		} else if len(key) > 0 {
			if err := s.openData(key); err == nil {
				s.remembered = true
			} else {
				s.warning = "本机记录未能解锁此密钥库，请输入主密码。"
			}
			clear(key)
		}
	}
	return s, nil
}
func (s *Store) openData(key []byte) error {
	if len(key) != 32 {
		return ErrLocked
	}
	a, err := gcm(key)
	if err != nil {
		return err
	}
	p, err := a.Open(nil, nil, s.env.Payload, s.env.aad(true))
	if err != nil {
		return errors.New("密钥库内容校验失败，原文件已保留")
	}
	defer clear(p)
	var values map[string]Entry
	if json.Unmarshal(p, &values) != nil || values == nil {
		return errors.New("密钥库内容无效")
	}
	s.entries = values
	s.key = bytes.Clone(key)
	return nil
}
func (s *Store) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Configured: s.env.Version != 0, Unlocked: s.key != nil, Remembered: s.remembered, Warning: s.warning}
}
func ValidatePassword(pass string) error {
	if !utf8.ValidString(pass) || utf8.RuneCountInString(pass) < 4 || len(pass) > 1024 {
		return errors.New("密码至少需要 4 个字符，且不能超过 1024 字节")
	}
	return nil
}
func (s *Store) Create(pass string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.env.Version != 0 {
		return errors.New("密钥库已存在，请解锁")
	}
	if err := ValidatePassword(pass); err != nil {
		return err
	}
	id, salt, key := make([]byte, 16), make([]byte, 16), make([]byte, 32)
	rand.Read(id)
	rand.Read(salt)
	rand.Read(key)
	defer clear(key)
	e := envelope{Version: 1, ID: hex.EncodeToString(id), Salt: salt}
	kek := derive(pass, salt)
	defer clear(kek)
	a, err := gcm(kek)
	if err != nil {
		return err
	}
	e.Wrapped = a.Seal(nil, nil, key, e.aad(false))
	values := map[string]Entry{}
	if err = s.save(e, key, values); err != nil {
		return err
	}
	s.key = bytes.Clone(key)
	return nil
}
func (s *Store) Unlock(pass string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.env.Version == 0 {
		return ErrMissing
	}
	if pass == "" || len(pass) > 1024 {
		return errors.New("主密码无效")
	}
	kek := derive(pass, s.env.Salt)
	defer clear(kek)
	a, err := gcm(kek)
	if err != nil {
		return err
	}
	key, err := a.Open(nil, nil, s.env.Wrapped, s.env.aad(false))
	if err != nil {
		return errors.New("主密码不正确或密钥库已损坏")
	}
	defer clear(key)
	return s.openData(key)
}
func (s *Store) Remember(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return ErrLocked
	}
	if s.device == nil {
		if !on {
			return nil
		}
		return errors.New("本机没有可用的系统凭据库")
	}
	var err error
	if on {
		err = s.device.Save(s.env.ID, s.key)
	} else {
		err = s.device.Forget(s.env.ID)
	}
	if err != nil {
		return fmt.Errorf("无法更新本机的系统凭据：%w", err)
	}
	s.remembered = on
	s.warning = ""
	return nil
}
func (s *Store) Get(ref string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return Entry{}, ErrLocked
	}
	v, ok := s.entries[ref]
	if !ok {
		return Entry{}, ErrNoEntry
	}
	return v, nil
}

// Apply durably writes credentials before publishing redacted metadata. A failed metadata
// commit restores the previous credentials. Retrying a migration never replaces a newer key.
func (s *Store) Apply(changes map[string]Entry, migration bool, commit func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.env.Version == 0 {
		return ErrMissing
	}
	if s.key == nil {
		return ErrLocked
	}
	old := s.entries
	next := maps.Clone(old)
	for ref, v := range changes {
		if migration {
			if prev, ok := old[ref]; ok && prev != v {
				return errors.New("已有凭据与旧配置不同，已保留两份数据")
			}
		}
		if v.Value == "" {
			delete(next, ref)
		} else {
			next[ref] = v
		}
	}
	changed := !maps.Equal(old, next)
	if changed {
		if err := s.save(s.env, s.key, next); err != nil {
			return err
		}
	}
	if commit != nil {
		if err := commit(); err != nil {
			if changed {
				return errors.Join(err, s.save(s.env, s.key, old))
			}
			return err
		}
	}
	return nil
}
func (s *Store) save(e envelope, key []byte, values map[string]Entry) error {
	a, err := gcm(key)
	if err != nil {
		return err
	}
	p, err := json.Marshal(values)
	if err != nil {
		return err
	}
	defer clear(p)
	e.Payload = a.Seal(nil, nil, p, e.aad(true))
	if e.Models, err = sealSharedModels(e, key, values); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(data) > maxFile {
		return errors.New("密钥库过大")
	}
	if err := s.fs.CreateDirectory(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	// Refuse to replace another instance's file or a backup restored while CAF was open.
	current, err := s.fs.ReadFile(s.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if (s.disk == nil) != errors.Is(err, fs.ErrNotExist) || !bytes.Equal(current, s.disk) {
		return errors.New("密钥库已被其他进程更改，请重启 CAF 后重试")
	}
	tmp := s.path + "." + rand.Text() + ".tmp"
	if err = s.fs.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err = s.fs.MoveFile(tmp, s.path); err != nil {
		_ = s.fs.DeleteFile(tmp)
		return err
	}
	s.env, s.disk, s.entries = e, data, values
	return nil
}
