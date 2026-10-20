package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableDefaults(t *testing.T) {
	home := filepath.Join(t.TempDir(), "CAF 测试")
	p, err := FromExecutable(filepath.Join(home, "CAF.exe"), "")
	if err != nil || p.Home != home || p.Data != filepath.Join(home, "data") || p.Logs != filepath.Join(home, "data", "logs") {
		t.Fatalf("unexpected paths: %+v, %v", p, err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(p.Logs); err != nil || !info.IsDir() {
		t.Fatalf("logs directory was not created: %v", err)
	}
}

func TestOverridesAreNotRelativeToWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	for _, override := range []string{"portable-data", filepath.Join(t.TempDir(), "external-data")} {
		p, err := FromExecutable(filepath.Join(home, "CAF.exe"), override)
		want := override
		if !filepath.IsAbs(want) {
			want = filepath.Join(home, want)
		}
		if err != nil || p.Data != want {
			t.Fatalf("override %q: %+v, %v", override, p, err)
		}
	}
}

func TestRejectsRelativeExecutableAndData(t *testing.T) {
	if _, err := FromExecutable("CAF.exe", ""); err == nil {
		t.Fatal("relative executable accepted")
	}
	if err := (Paths{Data: "data", Logs: "logs"}).Ensure(); err == nil {
		t.Fatal("relative data paths accepted")
	}
}

func TestDataDirectoryCannotBeAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("keep intact"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (Paths{Data: file, Logs: filepath.Join(file, "logs")}).Ensure(); err == nil {
		t.Fatal("file accepted as data directory")
	}
}
