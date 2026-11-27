package credentials

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
)

// A grant selects one model from one specific provider.
type ModelGrant struct {
	ConfigID string `json:"configId"`
	ModelID  string `json:"modelId"`
}

const GrantPrefix = "grant/"

type sharedPayload struct {
	Grants map[string][]ModelGrant `json:"grants"`
	Keys   map[string]Entry        `json:"keys"`
}

func sharingKey(dataKey []byte) []byte {
	mac := hmac.New(sha256.New, dataKey)
	mac.Write([]byte("CAF/models/sharing-key/v1"))
	return mac.Sum(nil)
}
func (s *Store) ModelSharingKey() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return nil, ErrLocked
	}
	return sharingKey(s.key), nil
}
func modelsAAD(e envelope) []byte {
	digest := sha256.Sum256(e.Payload)
	return append([]byte("CAF/models/v1/"+e.ID+"/"), digest[:]...)
}

// The native model-only key cannot decrypt the primary payload or conversations. Build
// the projection on the same atomic save as grants and credentials, including rollbacks.
func sealSharedModels(e envelope, key []byte, entries map[string]Entry) ([]byte, error) {
	projection := sharedPayload{Grants: map[string][]ModelGrant{}, Keys: map[string]Entry{}}
	for ref, entry := range entries {
		user, ok := strings.CutPrefix(ref, GrantPrefix)
		if !ok {
			continue
		}
		var grants []ModelGrant
		if err := json.Unmarshal([]byte(entry.Value), &grants); err != nil {
			return nil, err
		}
		if len(grants) == 0 {
			continue
		}
		projection.Grants[user] = grants
		for _, g := range grants {
			if v, ok := entries["model/"+g.ConfigID]; ok {
				projection.Keys[g.ConfigID] = v
			}
		}
	}
	if len(projection.Grants) == 0 {
		return nil, nil
	}
	k := sharingKey(key)
	defer clear(k)
	a, err := gcm(k)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return a.Seal(nil, nil, raw, modelsAAD(e)), nil
}
func (s *Store) HasSharedModels() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.env.Models) != 0
}

// A locked owner vault can serve only the specific guest's model grants and model keys.
func (s *Store) SharedModels(key []byte, user string) (map[ModelGrant]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowed := map[ModelGrant]Entry{}
	if len(s.env.Models) == 0 {
		return allowed, nil
	}
	if len(key) != 32 {
		return nil, errors.New("请主用户先在此设备登录一次，以启用共享模型")
	}
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	raw, err := a.Open(nil, nil, s.env.Models, modelsAAD(s.env))
	if err != nil {
		return nil, errors.New("共享模型授权校验失败，请主用户重新登录")
	}
	defer clear(raw)
	var data sharedPayload
	if err = json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	for _, g := range data.Grants[user] {
		allowed[g] = data.Keys[g.ConfigID]
	}
	return allowed, nil
}
