package Tests

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	"github.com/caliniya/caf/core/credentials"
	platform "github.com/caliniya/caf/platform"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
)

const testVaultPass = "only a test passphrase"

// Never touch the user's native credential store in automated tests.
type testDeviceKeys struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (d *testDeviceKeys) Load(id string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return bytes.Clone(d.values[id]), nil
}
func (d *testDeviceKeys) Save(id string, b []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.values == nil {
		d.values = map[string][]byte{}
	}
	d.values[id] = bytes.Clone(b)
	return nil
}
func (d *testDeviceKeys) Forget(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.values, id)
	return nil
}
func setupTestVault(t *testing.T, c *backend.Core) {
	t.Helper()
	c.DeviceKeys = &testDeviceKeys{}
	if err := c.CreateVault(testVaultPass); err != nil {
		t.Fatal(err)
	}
	if err := c.SetVaultRemembered(true); err != nil {
		t.Fatal(err)
	}
}
func freshVaultHost(root string, keys credentials.DeviceKeys) *backend.Core {
	c := &backend.Core{Paths: &platform.Paths{Data: root, ConfigFile: filepath.Join(root, "config", "MainConfig.json")}, FS: &desktop.DesktopFileSystem{}, DeviceKeys: keys}
	c.InitServers()
	return c
}
func TestVaultMigrationAndOfflineTransfer(t *testing.T) {
	device := &testDeviceKeys{}
	src := freshVaultHost(t.TempDir(), device)
	providerKey, qqToken := "provider-"+rand.Text(), "qq-"+rand.Text()
	var requests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+providerKey {
			t.Error("model request did not use the original key")
		}
		requests.Add(1)
		fmt.Fprint(w, `{"data":[{"id":"m"}]}`)
	}))
	defer model.Close()
	entry := map[string]any{"ID": "original-provider", "ServerAddress": model.URL, "Model": []string{"m"}, "APIKey": providerKey}
	config, _ := json.Marshal(map[string]any{"config": []any{entry}, "EnablesConfig": entry})
	if err := os.MkdirAll(filepath.Dir(src.Paths.ConfigFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src.Paths.ConfigFile, config, 0600); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	qqPath := filepath.Join(src.Paths.Data, "qq", "adapters.json")
	if err := os.MkdirAll(filepath.Dir(qqPath), 0700); err != nil {
		t.Fatal(err)
	}
	qqFile, _ := json.Marshal(map[string]any{"version": 1, "adapters": []any{map[string]any{"id": "original-qq", "name": "Bot", "port": port, "token": qqToken}}})
	if err = os.WriteFile(qqPath, qqFile, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := src.LoadConfigs(); !errors.Is(err, credentials.ErrMissing) {
		t.Fatal("legacy config modified without a vault", err)
	}
	if _, err := src.ListQQAdapters(); !errors.Is(err, credentials.ErrMissing) {
		t.Fatal("legacy QQ config modified without a vault", err)
	}
	assertRecoveryContent(t, src.Paths.ConfigFile, string(config))
	assertRecoveryContent(t, qqPath, string(qqFile))
	if err = src.CreateVault(testVaultPass); err != nil {
		t.Fatal(err)
	}
	if err = src.StartVaultSession(); err != nil {
		t.Fatal(err)
	}
	if err = src.SetVaultRemembered(true); err != nil {
		t.Fatal(err)
	}
	if err = filepath.WalkDir(src.Paths.Data, func(path string, d fs.DirEntry, e error) error {
		if e != nil || d.IsDir() {
			return e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		for _, secret := range []string{providerKey, qqToken, testVaultPass} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("plaintext in %s", filepath.Base(path))
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := src.LoadConfigs()
	if err != nil {
		t.Fatal(err)
	}
	args, err := src.GetExtensionParameters("original-qq")
	if err != nil {
		t.Fatal(err)
	}
	js, _ := json.Marshal([]any{cfg, args})
	if bytes.Contains(js, []byte(providerKey)) || bytes.Contains(js, []byte(qqToken)) || args["token"] != "" || !cfg.EnablesConfig.HasAPIKey {
		t.Fatal("edit API exposed credentials or lost their status")
	}
	if old, e := freshVaultHost(src.Paths.Data, device).GetVaultStatus(); e != nil || !old.Remembered || !old.Unlocked {
		t.Fatalf("remembered device not restored: %+v / %v", old, e)
	}
	dst := freshVaultHost(t.TempDir(), &testDeviceKeys{})
	if err = os.CopyFS(dst.Paths.Data, os.DirFS(src.Paths.Data)); err != nil {
		t.Fatal(err)
	}
	st, err := dst.GetVaultStatus()
	if err != nil || !st.Configured || st.Unlocked || st.Remembered {
		t.Fatal("copied directory should require a passphrase", st, err)
	}
	if err := dst.UnlockVault("wrong passphrase"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if err := dst.StartVaultSession(); !errors.Is(err, credentials.ErrLocked) {
		t.Fatal("locked host activated", err)
	}
	if _, err = dst.StartQQAdapter("original-qq"); !errors.Is(err, credentials.ErrLocked) {
		t.Fatal("locked adapter could start", err)
	}
	if err = dst.UnlockVault(testVaultPass); err != nil {
		t.Fatal(err)
	}
	if err = dst.StartVaultSession(); err != nil {
		t.Fatal(err)
	}
	if _, err = dst.ProbeSavedModels("original-provider", model.URL); err != nil || requests.Load() == 0 {
		t.Fatal("copied credential did not authenticate", err)
	}
	qq := testServer[*backend.QQServer](t, dst, backend.QQServerName)
	qq.ListenHost = "127.0.0.1"
	started, err := dst.StartQQAdapter("original-qq")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dst.CloseQQAdapters)
	connectReverseWS(t, started.Address, "10001", qqToken)
	waitQQAdapter(t, dst, started.ID, "connected", "10001")
	if err = src.SetVaultRemembered(false); err != nil {
		t.Fatal(err)
	}
	st, err = freshVaultHost(src.Paths.Data, device).GetVaultStatus()
	if err != nil || st.Unlocked {
		t.Fatal("forget did not take effect on restart", st, err)
	}
}
func TestVaultTamperAndFailedWrites(t *testing.T) {
	root := t.TempDir()
	device := &testDeviceKeys{}
	c := freshVaultHost(root, device)
	if err := c.CreateVault(testVaultPass); err != nil {
		t.Fatal(err)
	}
	if err := c.SetVaultRemembered(true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "secrets.vault")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"wrapped", "salt", "payload"} {
		var env map[string]any
		if err = json.Unmarshal(original, &env); err != nil {
			t.Fatal(err)
		}
		raw, _ := base64.StdEncoding.DecodeString(env[part].(string))
		raw[0] ^= 1
		env[part] = base64.StdEncoding.EncodeToString(raw)
		damaged, _ := json.Marshal(env)
		if err = os.WriteFile(path, damaged, 0600); err != nil {
			t.Fatal(err)
		}
		other := freshVaultHost(root, device)
		if st, e := other.GetVaultStatus(); e != nil || st.Unlocked {
			t.Fatal("tampered vault auto-unlocked", part, e)
		}
		if err = other.UnlockVault(testVaultPass); err == nil {
			t.Fatal("tampered vault unlocked", part)
		}
		if err = other.CreateVault(testVaultPass); err == nil {
			t.Fatal("existing vault overwritten")
		}
		assertRecoveryContent(t, path, string(damaged))
	}
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	v, err := credentials.Open(c.FS, path, device)
	if err != nil {
		t.Fatal(err)
	}
	err = v.Apply(map[string]credentials.Entry{"entry": {Value: "must-not-be-committed"}}, false, func() error { return fs.ErrPermission })
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal("failed metadata write ignored", err)
	}
	reopened, e := credentials.Open(c.FS, path, device)
	if e != nil {
		t.Fatal(e)
	}
	if _, err = reopened.Get("entry"); !errors.Is(err, credentials.ErrNoEntry) {
		t.Fatal("failed metadata write left an active credential", err)
	}
}
func TestVaultOriginBindingAndRotation(t *testing.T) {
	c := freshVaultHost(t.TempDir(), &testDeviceKeys{})
	if e := c.CreateVault(testVaultPass); e != nil {
		t.Fatal(e)
	}
	if e := c.StartVaultSession(); e != nil {
		t.Fatal(e)
	}
	var seen atomic.Int32
	var anonymous atomic.Bool
	two := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "Bearer replacement-token"
		if anonymous.Load() {
			want = ""
		}
		if r.Header.Get("Authorization") != want {
			t.Error("request did not use the credential selected for this origin")
		}
		seen.Add(1)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer two.Close()
	one := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, two.URL, http.StatusTemporaryRedirect)
	}))
	defer one.Close()
	models := []api.Model{{ID: "m"}}
	cfg, e := c.AddModelConfig("test", one.URL, "", "original-token", models)
	if e != nil {
		t.Fatal(e)
	}
	id := cfg.EnablesConfig.ID
	if _, e = c.ProbeSavedModels(id, one.URL); e != nil {
		t.Fatal(e)
	}
	if _, e = c.ProbeSavedModels(id, two.URL); e == nil {
		t.Fatal("stored credential allowed wrong origin")
	}
	if seen.Load() != 0 {
		t.Fatal("cross-origin request sent")
	}
	if _, e = c.RefreshModelConfig(0, two.URL, models); e == nil {
		t.Fatal("editing a URL silently rebound a key")
	}
	if _, e = c.UpdateModelCredentials(0, two.URL, models, "replacement-token", false); e != nil {
		t.Fatal(e)
	}
	if _, e = c.ProbeSavedModels(id, two.URL); e != nil || seen.Load() == 0 {
		t.Fatal("replacement credential not used", e)
	}
	cfg, e = c.UpdateModelCredentials(0, two.URL, models, "", true)
	if e != nil || cfg.EnablesConfig.HasAPIKey {
		t.Fatal("clear key failed", e)
	}
	anonymous.Store(true)
	before := seen.Load()
	if _, e = c.ProbeSavedModels(id, two.URL); e != nil || seen.Load() <= before {
		t.Fatal("provider no longer works with its key cleared", e)
	}
	b, _ := os.ReadFile(c.Paths.ConfigFile)
	if strings.Contains(string(b), "-token") {
		t.Fatal("credential leaked to metadata")
	}
}

func TestVaultMigratesEnabledOnlyKeyForOriginalOrigin(t *testing.T) {
	c := freshVaultHost(t.TempDir(), &testDeviceKeys{})
	setupTestVault(t, c)
	if err := os.MkdirAll(filepath.Dir(c.Paths.ConfigFile), 0700); err != nil {
		t.Fatal(err)
	}
	writeLegacy := func(address, key string) []byte {
		t.Helper()
		list := map[string]any{"ID": "provider", "ServerAddress": "https://original.example/v1", "Model": []string{"m"}}
		enabled := map[string]any{"ID": "provider", "ServerAddress": address, "APIKey": key, "Model": []string{"m"}}
		data, err := json.Marshal(map[string]any{"config": []any{list}, "EnablesConfig": enabled})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(c.Paths.ConfigFile, data, 0600); err != nil {
			t.Fatal(err)
		}
		return data
	}
	key := rand.Text()
	original := writeLegacy("https://other.example/v1", key)
	if _, err := c.LoadConfigs(); err == nil {
		t.Fatal("rebound an enabled-only legacy key to another host")
	}
	assertRecoveryContent(t, c.Paths.ConfigFile, string(original))
	writeLegacy("https://original.example:443/chat/completions", key)
	if cfg, err := c.LoadConfigs(); err != nil || !cfg.EnablesConfig.HasAPIKey {
		t.Fatal("enabled-only key did not migrate", err)
	}
	conflict := writeLegacy("https://original.example", rand.Text())
	if _, err := c.LoadConfigs(); err == nil {
		t.Fatal("a stale config replaced an existing vault key")
	}
	assertRecoveryContent(t, c.Paths.ConfigFile, string(conflict))
	vault, err := credentials.Open(c.FS, filepath.Join(c.Paths.Data, "secrets.vault"), c.DeviceKeys)
	if err != nil {
		t.Fatal(err)
	}
	if entry, err := vault.Get("model/provider"); err != nil || entry.Value != key {
		t.Fatal("did not retain the migrated key", err)
	}
}
