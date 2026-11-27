package credentials

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func (s *Store) ID() string { s.mu.Lock(); defer s.mu.Unlock(); return s.env.ID }

func (s *Store) unwrap(pass string) ([]byte, bool) {
	if pass == "" || len(pass) > 1024 || s.env.Version == 0 {
		return nil, false
	}
	kek := derive(pass, s.env.Salt)
	defer clear(kek)
	a, err := gcm(kek)
	if err != nil {
		return nil, false
	}
	key, err := a.Open(nil, nil, s.env.Wrapped, s.env.aad(false))
	return key, err == nil
}

// Matches checks a password without opening or caching that profile's secrets.
func (s *Store) Matches(pass string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.unwrap(pass)
	clear(key)
	return ok
}
func (s *Store) ChangePassword(old, next string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return ErrLocked
	}
	if err := ValidatePassword(next); err != nil {
		return err
	}
	key, ok := s.unwrap(old)
	defer clear(key)
	if !ok || !hmac.Equal(key, s.key) {
		return errors.New("当前密码不正确")
	}
	e := s.env
	e.Salt = make([]byte, 16)
	rand.Read(e.Salt)
	kek := derive(next, e.Salt)
	defer clear(kek)
	a, err := gcm(kek)
	if err != nil {
		return err
	}
	e.Wrapped = a.Seal(nil, nil, key, e.aad(false))
	return s.save(e, key, s.entries)
}
func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.key)
	s.key = nil
	clear(s.entries)
	s.entries = nil
	s.remembered = false
}

// One native entry holds the selected profile and its data key; replacing it switches defaults atomically.
func (s *Store) SaveDefault(slot string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return ErrLocked
	}
	if s.device == nil {
		return errors.New("本机没有可用的系统凭据库")
	}
	record, _ := hex.DecodeString(s.env.ID)
	record = append(record, s.key...)
	defer clear(record)
	return s.device.Save(slot, record)
}

// Explicit logout replaces the auto-login key with just the public profile ID.
func (s *Store) SuspendDefault(slot string) error {
	if s.device == nil {
		return errors.New("本机没有可用的系统凭据库")
	}
	id, _ := hex.DecodeString(s.ID())
	return s.device.Save(slot, id)
}

// The 32-byte legacy record always belongs to the first (owner) profile.
func DeviceDefault(record []byte, ownerID string) (id string, suspended bool) {
	switch len(record) {
	case 16:
		return hex.EncodeToString(record), true
	case 48:
		return hex.EncodeToString(record[:16]), false
	case 32:
		return ownerID, false
	default:
		return "", false
	}
}
func (s *Store) UnlockDefault(record []byte, primary bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, _ := hex.DecodeString(s.env.ID)
	switch {
	case len(record) == 48 && bytes.Equal(record[:16], id):
		return true, s.openData(record[16:])
	case len(record) == 32 && primary:
		return true, s.openData(record)
	} // pre-profile device entry
	return false, nil
}

const chatMagic = "CAFchat\x00\x01"

func IsEncryptedChat(b []byte) bool { return bytes.HasPrefix(b, []byte(chatMagic)) }
func (s *Store) ChatBytes(scope string, data []byte, encrypt bool) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return nil, ErrLocked
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("CAF/chat/data-key/v1"))
	key := mac.Sum(nil)
	defer clear(key)
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	aad := []byte(chatMagic + s.env.ID + "/" + scope)
	if encrypt {
		return a.Seal([]byte(chatMagic), nil, data, aad), nil
	}
	if !IsEncryptedChat(data) {
		return nil, errors.New("对话文件不是有效的加密数据")
	}
	result, err := a.Open(nil, nil, data[len(chatMagic):], aad)
	if err != nil {
		return nil, errors.New("对话文件校验失败，原文件已保留")
	}
	return result, nil
}
