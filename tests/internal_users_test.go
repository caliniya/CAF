package Tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	"github.com/caliniya/caf/core/credentials"
)

func TestInternalUserSharedModels(t *testing.T) {
	const ownerPass = "四字口令"
	const guestPass = "1234"
	const first, next = "test-shared-key", "test-rotated-key"
	base := t.TempDir()
	picker := &profilePicker{dest: t.TempDir()}
	keys := &testDeviceKeys{}
	a := openProfiles(t, base, picker, keys)
	if e := a.CreateVault("甲乙丙"); e == nil {
		t.Fatal("accepted three Unicode characters")
	}
	checkOK(t, a.CreateVault(ownerPass))
	ticket := profileStart(t, a)
	var hits atomic.Int32
	var expected atomic.Value
	expected.Store(first)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.Header.Get("Authorization") != "Bearer "+expected.Load().(string) || body.Model != "common" {
			t.Error("incorrect credentials or model")
		}
		hits.Add(1)
		qqSSE(w, "shared reply")
	}))
	defer server.Close()
	models := []api.Model{{ID: "common"}, {ID: "second"}, {ID: "private"}}
	add := func(name, key string) string {
		t.Helper()
		var cfg backend.Configs
		checkOK(t, json.Unmarshal([]byte(profileCall(t, a, ticket, "AddModelConfig", name, server.URL, "", key, models)), &cfg))
		return cfg.EnablesConfig.ID
	}
	one, two := add("first", first), add("second provider", "never-share-this-key")
	common := credentials.ModelGrant{ConfigID: one, ModelID: "common"}
	second := credentials.ModelGrant{ConfigID: one, ModelID: "second"}
	if _, err := a.CreateInternalUser(ticket, "短密码", nil); err == nil {
		t.Fatal("accepted a three-character guest passphrase")
	}
	if _, err := a.CreateInternalUser(ticket, guestPass, []credentials.ModelGrant{{ConfigID: two, ModelID: "absent"}}); err == nil {
		t.Fatal("unknown grant accepted")
	}
	user, err := a.CreateInternalUser(ticket, guestPass, []credentials.ModelGrant{common, second, common})
	checkOK(t, err)
	users, err := a.ListInternalUsers(ticket)
	checkOK(t, err)
	if len(users) != 1 || len(user.Models) != 2 || users[0].ID != user.ID {
		t.Fatal("failed creation leaked a guest, or duplicate grant")
	}
	if err := a.ChangePassword(ticket, ownerPass, "三个字"); err == nil {
		t.Fatal("accepted a short new passphrase")
	}
	checkOK(t, a.ChangePassword(ticket, ownerPass, "新版密码"))
	checkOK(t, a.Logout(ticket))
	// A pre-existing secondary profile may already own a provider with a colliding ID.
	// Keep its original settings for its export, but do not use them for routing or grants.
	dirs, err := os.ReadDir(filepath.Join(base, "profiles"))
	checkOK(t, err)
	legacyPath := filepath.Join(base, "profiles", dirs[0].Name(), "config", "MainConfig.json")
	legacy := map[string]any{"ID": two, "Name": "old private provider", "ServerAddress": "https://legacy.invalid", "Model": []string{"private"}, "EnabledModel": "private", "APIKey": "guest-legacy-secret"}
	legacyFile, err := json.Marshal(map[string]any{"config": []any{legacy}, "EnablesConfig": legacy})
	checkOK(t, err)
	checkOK(t, os.MkdirAll(filepath.Dir(legacyPath), 0700))
	checkOK(t, os.WriteFile(legacyPath, legacyFile, 0600))
	// A cold restart must serve shared models even when the owner is not remembered or logged in.
	a = openProfiles(t, base, picker, keys)
	checkOK(t, a.UnlockVault(guestPass))
	guest := profileStart(t, a)
	list := profileCall(t, a, guest, "LoadConfigs")
	if !strings.Contains(list, "common") || !strings.Contains(list, "\"sharedModels\":true") || strings.Contains(list, two) || strings.Contains(list, "private") || strings.Contains(list, server.URL) || strings.Contains(list, first) {
		t.Fatal("incorrect shared catalogue")
	}
	if _, err = a.ListInternalUsers(guest); err == nil {
		t.Fatal("guest listed users")
	}
	if _, err = a.CreateInternalUser(guest, "other pass", nil); err == nil {
		t.Fatal("guest created an account")
	}
	if _, err = a.SetInternalUserModels(guest, user.ID, []credentials.ModelGrant{{ConfigID: two, ModelID: "common"}}); err == nil {
		t.Fatal("guest escalated grants")
	}
	deny := func(method string, args ...any) {
		t.Helper()
		b, _ := json.Marshal(args)
		if _, e := a.Call(guest, method, string(b)); e == nil {
			t.Fatal("unexpected permission for", method)
		}
	}
	deny("CreateChat", two, "common", "", "")
	deny("CreateChat", one, "private", "", "")
	deny("SelectModel", "private")
	deny("ProbeSavedModels", one, server.URL)
	deny("ProbeModels", server.URL, "")
	deny("DeleteConfig", 0)
	deny("RenameConfig", 0, "rename")
	deny("RefreshModelConfig", 0, server.URL, models)
	deny("UpdateModelCredentials", 0, server.URL, models, "", true)
	deny("AddModelConfig", "custom", server.URL, "", "", models)
	if hits.Load() != 0 {
		t.Fatal("unauthorized request reached upstream")
	}
	profileCall(t, a, guest, "SelectModel", "second")
	var prefs backend.Configs
	checkOK(t, json.Unmarshal([]byte(profileCall(t, a, guest, "LoadConfigs")), &prefs))
	if prefs.EnablesConfig.EnabledModel != "second" {
		t.Fatal("lost personal selection")
	}
	var chat struct {
		ID string `json:"id"`
	}
	checkOK(t, json.Unmarshal([]byte(profileCall(t, a, guest, "CreateChat", one, "common", "guest chat", "")), &chat))
	profileCall(t, a, guest, "SendChatMessage", chat.ID, "hello", "")
	if hits.Load() != 1 || !strings.Contains(profileCall(t, a, guest, "GetChat", chat.ID), "shared reply") {
		t.Fatal("allowed request failed")
	}
	archive, err := a.ExportUserData(guest, guestPass, true)
	checkOK(t, err)
	filesInZip := zipFiles(t, archive)
	if !strings.Contains(filesInZip["config/MainConfig.json"], "old private provider") {
		t.Fatal("lost old personal provider configuration")
	}
	for name, b := range filesInZip {
		if strings.Contains(b, "guest-legacy-secret") || strings.Contains(b, first) || strings.Contains(b, server.URL) || strings.Contains(name, "secrets.vault") {
			t.Fatal("guest export leaked provider details", name)
		}
	}
	checkOK(t, a.Logout(guest))
	checkOK(t, a.UnlockVault("新版密码"))
	ticket = profileStart(t, a)
	profileCall(t, a, ticket, "UpdateModelCredentials", 0, server.URL, models, next, false)
	expected.Store(next)
	checkOK(t, a.Logout(ticket))
	checkOK(t, a.UnlockVault(guestPass))
	guest = profileStart(t, a)
	profileCall(t, a, guest, "SendChatMessage", chat.ID, "again", "")
	if hits.Load() != 2 {
		t.Fatal("rotated key not usable")
	}
	checkOK(t, a.Logout(guest))
	checkOK(t, a.UnlockVault("新版密码"))
	ticket = profileStart(t, a)
	_, err = a.SetInternalUserModels(ticket, user.ID, []credentials.ModelGrant{second})
	checkOK(t, err)
	checkOK(t, a.Logout(ticket))
	checkOK(t, a.UnlockVault(guestPass))
	guest = profileStart(t, a)
	deny("SendChatMessage", chat.ID, "after revocation", "")
	if hits.Load() != 2 || !strings.Contains(profileCall(t, a, guest, "GetChat", chat.ID), "shared reply") {
		t.Fatal("revocation should block spending but preserve history")
	}
	checkOK(t, a.Logout(guest))
	checkOK(t, a.UnlockVault("新版密码"))
	ticket = profileStart(t, a)
	profileCall(t, a, ticket, "DeleteConfig", 0)
	checkOK(t, a.Logout(ticket))
	checkOK(t, a.UnlockVault(guestPass))
	guest = profileStart(t, a)
	if list := profileCall(t, a, guest, "LoadConfigs"); strings.Contains(list, one) {
		t.Fatal("removed provider still available")
	}
	deny("CreateChat", "", "", "", "")
	if hits.Load() != 2 {
		t.Fatal("unexpected request")
	}
}

func TestCopiedInternalProfileWithoutDeviceRecord(t *testing.T) {
	root := t.TempDir()
	files := &profilePicker{}
	a := openProfiles(t, root, files, nil)
	checkOK(t, a.CreateVault("owner pass"))
	tkt := profileStart(t, a)
	var cfg backend.Configs
	checkOK(t, json.Unmarshal([]byte(profileCall(t, a, tkt, "AddModelConfig", "home", "http://localhost:10000", "", "local-test-key", []api.Model{{ID: "shared"}})), &cfg))
	grant := credentials.ModelGrant{ConfigID: cfg.EnablesConfig.ID, ModelID: "shared"}
	_, err := a.CreateInternalUser(tkt, "guest pass", []credentials.ModelGrant{grant})
	checkOK(t, err)
	_, err = a.CreateInternalUser(tkt, "other pass", nil)
	checkOK(t, err)
	checkOK(t, a.Logout(tkt))
	checkOK(t, a.UnlockVault("other pass"))
	tkt = profileStart(t, a)
	if raw := profileCall(t, a, tkt, "LoadConfigs"); strings.Contains(raw, grant.ConfigID) {
		t.Fatal("another internal user inherited the grant")
	}
	checkOK(t, a.Logout(tkt))
	checkOK(t, a.UnlockVault("guest pass"))
	tkt = profileStart(t, a)
	if !strings.Contains(profileCall(t, a, tkt, "LoadConfigs"), grant.ConfigID) {
		t.Fatal("no in-process sharing without a keyring")
	}
	checkOK(t, a.Logout(tkt))
	dest := t.TempDir()
	checkOK(t, os.CopyFS(dest, os.DirFS(root)))
	keys := &testDeviceKeys{}
	copied := openProfiles(t, dest, files, keys)
	checkOK(t, copied.UnlockVault("guest pass"))
	guest := profileStart(t, copied)
	var empty backend.Configs
	checkOK(t, json.Unmarshal([]byte(profileCall(t, copied, guest, "LoadConfigs")), &empty))
	if !empty.SharedModels || len(*empty.ALLConfig) != 0 || empty.SharedModelNotice == "" {
		t.Fatal("fresh device did not fail closed with an explanation")
	}
	checkOK(t, copied.Logout(guest))
	checkOK(t, copied.UnlockVault("owner pass"))
	tkt = profileStart(t, copied)
	checkOK(t, copied.Logout(tkt))
	owner, err := credentials.OpenLocked(files, filepath.Join(dest, "secrets.vault"), nil)
	checkOK(t, err)
	modelKey, err := keys.Load("models-v1/" + owner.ID())
	checkOK(t, err)
	if len(modelKey) != 32 {
		t.Fatal("no model service key on new device")
	}
	if _, err := owner.UnlockDefault(modelKey, true); err == nil || owner.Status().Unlocked {
		t.Fatal("model sharing unlocked the owner vault")
	}
	cold := openProfiles(t, dest, files, keys)
	checkOK(t, cold.UnlockVault("guest pass"))
	guest = profileStart(t, cold)
	if !strings.Contains(profileCall(t, cold, guest, "LoadConfigs"), grant.ConfigID) {
		t.Fatal("device transfer lost authorized models")
	}
}
