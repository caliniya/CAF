package Tests

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	backend "github.com/caliniya/caf/core/Backend"
	"github.com/caliniya/caf/core/credentials"
	platform "github.com/caliniya/caf/platform"
	desktop "github.com/caliniya/caf/platform/backend/desktop"
)

type profilePicker struct {
	desktop.DesktopFileSystem
	dest    string
	entered chan struct{}
	release chan struct{}
}

func (f *profilePicker) SelectDirectory(string) (string, error) {
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	return f.dest, nil
}
func openProfiles(t *testing.T, base string, files platform.FilePlatform, keys credentials.DeviceKeys) *backend.Accounts {
	t.Helper()
	return profileManager(t, base, files, keys).API()
}
func profileManager(t *testing.T, base string, files platform.FilePlatform, keys credentials.DeviceKeys) *backend.Profiles {
	t.Helper()
	p := backend.NewProfiles(platform.Paths{Data: base}, files, keys, nil)
	t.Cleanup(func() {
		if e := p.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return p
}
func profileCall(t *testing.T, a *backend.Accounts, ticket, name string, args ...any) string {
	t.Helper()
	b, _ := json.Marshal(args)
	out, e := a.Call(ticket, name, string(b))
	if e != nil {
		t.Fatalf("%s: %v", name, e)
	}
	return string(out)
}
func profileStart(t *testing.T, a *backend.Accounts) string {
	t.Helper()
	v, e := a.StartVaultSession()
	if e != nil || v == "" {
		t.Fatalf("start: %v", e)
	}
	return v
}
func checkOK(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func zipFiles(t *testing.T, path string) map[string]string {
	t.Helper()
	z, e := zip.OpenReader(path)
	checkOK(t, e)
	defer z.Close()
	files := map[string]string{}
	for _, f := range z.File {
		r, e := f.Open()
		checkOK(t, e)
		b, e := io.ReadAll(r)
		checkOK(t, e)
		r.Close()
		files[f.Name] = string(b)
	}
	return files
}
func TestProfilesMigrationRotationDefaultAndExport(t *testing.T) {
	const aPass = "first local phrase"
	const next = "changed local phrase"
	const bPass = "second local phrase"
	const title = "private legacy title"
	const token = "test-qq-original"
	const modelKey = "test-provider-original"
	base := t.TempDir()
	keys := &testDeviceKeys{}
	picker := &profilePicker{dest: t.TempDir()}
	old := freshVaultHost(base, keys)
	checkOK(t, old.CreateVault(aPass))
	checkOK(t, old.StartVaultSession())
	checkOK(t, old.SetVaultRemembered(true))
	_, e := old.AddModelConfig("one", "https://first.example", "", modelKey, []api.Model{{ID: "m"}})
	checkOK(t, e)
	// Seed a disabled legacy adapter without starting a listener.
	checkOK(t, os.MkdirAll(filepath.Join(base, "qq"), 0700))
	checkOK(t, os.WriteFile(filepath.Join(base, "qq", "adapters.json"), []byte(`{"version":1,"adapters":[{"id":"orig-qq","name":"QQ","port":16321,"token":"`+token+`"}]}`), 0600))
	chat, e := old.CreateChat("", "", title, "")
	checkOK(t, e)
	chatRel := "chats/" + chat.ID + ".json"
	history := "ext/clients/bot/qq/adapter/history/chat/archive.json"
	body := `{"qq":"private archived message"}`
	hp := filepath.Join(base, filepath.FromSlash(history))
	checkOK(t, os.MkdirAll(filepath.Dir(hp), 0700))
	checkOK(t, os.WriteFile(hp, []byte(body), 0600))
	a := openProfiles(t, base, picker, keys)
	s, e := a.GetVaultStatus()
	if e != nil || !s.Unlocked || !s.Remembered || s.Role != "owner" {
		t.Fatal("old remembered user was lost", s, e)
	}
	ticket := profileStart(t, a)
	if !strings.Contains(profileCall(t, a, ticket, "GetChat", chat.ID), title) {
		t.Fatal("original conversation lost")
	}
	for _, path := range []string{chatRel, "chats/index.json", history} {
		b, e := os.ReadFile(filepath.Join(base, filepath.FromSlash(path)))
		checkOK(t, e)
		if !credentials.IsEncryptedChat(b) || bytes.Contains(b, []byte(title)) || bytes.Contains(b, []byte("private archived message")) {
			t.Fatal("unencrypted conversation", path)
		}
	}
	orig, e := os.ReadFile(filepath.Join(base, filepath.FromSlash(chatRel)))
	checkOK(t, e)
	if e := a.ChangePassword(ticket, "wrong", next); e == nil {
		t.Fatal("changed password without current password")
	}
	checkOK(t, a.ChangePassword(ticket, aPass, next))
	after, e := os.ReadFile(filepath.Join(base, filepath.FromSlash(chatRel)))
	checkOK(t, e)
	if !bytes.Equal(orig, after) {
		t.Fatal("password rotation rewrote chat")
	}
	if _, e = a.CreateInternalUser(ticket, next, nil); e == nil {
		t.Fatal("accepted a duplicate login")
	}
	_, e = a.CreateInternalUser(ticket, bPass, []credentials.ModelGrant{{ConfigID: chat.ConfigID, ModelID: chat.Model}})
	checkOK(t, e)
	if _, e = a.Call(ticket, "GetChat", `["`+chat.ID+`"]`); e != nil {
		t.Fatal("creating a guest signed out the owner", e)
	}
	checkOK(t, a.Logout(ticket))
	s, e = a.GetVaultStatus()
	if e != nil || s.Unlocked {
		t.Fatal("logout immediately unlocked", s, e)
	}
	if _, e = a.Call(ticket, "ListChats", "[]"); e == nil {
		t.Fatal("revoked ticket accepted")
	}
	if e = a.UnlockVault(aPass); e == nil {
		t.Fatal("old password accepted")
	}
	if e = a.CreateVault(next); e == nil {
		t.Fatal("duplicate password accepted")
	}
	if e = a.CreateVault("unregistered"); e == nil {
		t.Fatal("public registration still open")
	}
	if s, e := openProfiles(t, base, picker, keys).GetVaultStatus(); e != nil || s.Unlocked {
		t.Fatal("explicit logout allowed auto-login after restart", s, e)
	}
	checkOK(t, a.UnlockVault(bPass))
	bTicket := profileStart(t, a)
	if list := profileCall(t, a, bTicket, "ListChats"); strings.Contains(list, chat.ID) {
		t.Fatal("inherited another account's chats")
	}
	if _, e = a.Call(bTicket, "GetChat", `["`+chat.ID+`"]`); e == nil {
		t.Fatal("read other chat by ID")
	}
	// New writes use the second user's key from the first save, including the title index.
	raw := profileCall(t, a, bTicket, "CreateChat", "", "", "second private title", "")
	var second struct {
		ID string `json:"id"`
	}
	checkOK(t, json.Unmarshal([]byte(raw), &second))
	dirs, e := os.ReadDir(filepath.Join(base, "profiles"))
	checkOK(t, e)
	if len(dirs) != 1 {
		t.Fatal("expected one additional profile")
	}
	secondDir := filepath.Join(base, "profiles", dirs[0].Name())
	secondPath := filepath.Join(secondDir, "chats", second.ID+".json")
	cipher, e := os.ReadFile(secondPath)
	checkOK(t, e)
	if !credentials.IsEncryptedChat(cipher) || bytes.Contains(cipher, []byte("second private title")) {
		t.Fatal("new chat was saved in plaintext")
	}
	checkOK(t, os.WriteFile(secondPath, orig, 0600))
	if _, e = a.Call(bTicket, "GetChat", `["`+second.ID+`"]`); e == nil {
		t.Fatal("foreign ciphertext accepted")
	}
	checkOK(t, os.WriteFile(secondPath, cipher, 0600))
	if _, e = a.Call(bTicket, "CreateWorkspace", workspaceArgs(t, base)); e == nil {
		t.Fatal("another account's data root accepted as a workspace")
	}
	if e = a.ChangePassword(bTicket, bPass, next); e == nil {
		t.Fatal("rotated to another user's password")
	}
	checkOK(t, a.SetDefaultProfile(bTicket, true))
	checkOK(t, a.Logout(bTicket))
	mgr := profileManager(t, base, picker, keys)
	reboot := mgr.API()
	s, e = reboot.GetVaultStatus()
	if e != nil || s.Unlocked {
		t.Fatal("auto-login not suspended", s, e)
	}
	checkOK(t, reboot.UnlockVault(bPass))
	s, e = reboot.GetVaultStatus()
	if e != nil || s.Role != "internal" || !s.Remembered {
		t.Fatal("manual login did not restore selected default", s, e)
	}
	checkOK(t, mgr.Close(context.Background()))
	reboot = openProfiles(t, base, picker, keys)
	s, e = reboot.GetVaultStatus()
	if e != nil || !s.Unlocked || !s.Remembered || s.Role != "internal" {
		t.Fatal("normal restart lost the selected default", s, e)
	}
	bt := profileStart(t, reboot)
	if strings.Contains(profileCall(t, reboot, bt, "LoadConfigs"), "first.example") {
		t.Fatal("wrong default user")
	}
	checkOK(t, reboot.SetDefaultProfile(bt, false))
	checkOK(t, reboot.Logout(bt))
	s, e = openProfiles(t, base, picker, keys).GetVaultStatus()
	if e != nil || s.Unlocked {
		t.Fatal("clearing default did not persist", s, e)
	}
	checkOK(t, a.UnlockVault(next))
	ticket = profileStart(t, a)
	if _, e = a.ExportUserData(ticket, aPass, true); e == nil {
		t.Fatal("export allowed old password")
	}
	picker.dest = base
	if _, e = a.ExportUserData(ticket, next, false); e == nil {
		t.Fatal("accepted export inside live data")
	}
	picker.dest = t.TempDir()
	for _, include := range []bool{false, true} {
		path, e := a.ExportUserData(ticket, next, include)
		checkOK(t, e)
		files := zipFiles(t, path)
		if _, ok := files["chats/"+second.ID+".json"]; ok {
			t.Fatal("second account exported with the first")
		}
		if !strings.Contains(files[chatRel], title) || files[history] != body {
			t.Fatal("missing raw conversation")
		}
		if strings.Contains(files["config/MainConfig.json"], modelKey) != include || strings.Contains(files["qq/adapters.json"], token) != include {
			t.Fatal("export credential choice ignored")
		}
		for name := range files {
			if strings.HasPrefix(name, "profiles/") || strings.Contains(name, "secrets.vault") {
				t.Fatal("foreign profile or vault exported", name)
			}
		}
	}
	checkOK(t, a.Logout(ticket))
	dest := t.TempDir()
	checkOK(t, os.CopyFS(dest, os.DirFS(base)))
	other := openProfiles(t, dest, picker, &testDeviceKeys{})
	s, e = other.GetVaultStatus()
	if e != nil || s.Unlocked {
		t.Fatal("automatic login copied to another device", s, e)
	}
	checkOK(t, other.UnlockVault(next))
	copyTicket := profileStart(t, other)
	if !strings.Contains(profileCall(t, other, copyTicket, "GetChat", chat.ID), title) {
		t.Fatal("copied chat did not decrypt")
	}
	checkOK(t, other.Logout(copyTicket))
	checkOK(t, other.UnlockVault(bPass))
	secondTicket := profileStart(t, other)
	checkOK(t, other.Logout(secondTicket))
	// One unreadable secondary vault must not lock the owner out of the remaining profiles.
	checkOK(t, os.WriteFile(filepath.Join(dest, "profiles", dirs[0].Name(), "secrets.vault"), []byte("incomplete vault"), 0600))
	st, err := other.GetVaultStatus()
	if err != nil || !st.Configured || st.Warning == "" {
		t.Fatal("healthy profile blocked by unrelated damage", st, err)
	}
	checkOK(t, other.UnlockVault(next))
	profileStart(t, other)
}
func TestProfileLogoutDrainsAdmittedCalls(t *testing.T) {
	f := &profilePicker{entered: make(chan struct{}), release: make(chan struct{})}
	a := openProfiles(t, t.TempDir(), f, &testDeviceKeys{})
	checkOK(t, a.CreateVault("a local test pass"))
	ticket := profileStart(t, a)
	done := make(chan error, 1)
	go func() { _, e := a.Call(ticket, "SelectDirectory", `[""]`); done <- e }()
	<-f.entered
	closed := make(chan error, 1)
	go func() { closed <- a.Logout(ticket) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, e := a.Call(ticket, "ListPersonas", "[]"); e != nil {
			break
		}
		if time.Now().After(deadline) {
			close(f.release)
			t.Fatal("session was not revoked")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-closed:
		t.Fatal("logout returned before outstanding call completed")
	default:
	}
	close(f.release)
	if <-done == nil {
		t.Fatal("old session received a late result")
	}
	checkOK(t, <-closed)
}

func workspaceArgs(t *testing.T, path string) string {
	t.Helper()
	b, err := json.Marshal([]string{"private directory", path})
	checkOK(t, err)
	return string(b)
}
