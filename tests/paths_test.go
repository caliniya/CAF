package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/caliniya/caf/core/platform"
	"github.com/caliniya/caf/platforms/desktop"
)

func TestPortableDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CAF 测试")
	p, err := desktop.PathsFromExecutable(filepath.Join(root, "CAF.exe"), "")
	if err != nil || p.Root != root || p.Data != filepath.Join(root, "data") || p.Logs != filepath.Join(p.Data, "logs") {
		t.Fatalf("unexpected paths: %+v, %v", p, err)
	}
	if _, err := os.Stat(p.Data); !os.IsNotExist(err) {
		t.Fatal("path discovery created files", err)
	}
	b := desktop.New()
	defer b.Close()
	if err := b.Init(p); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(p.Logs); err != nil || !info.IsDir() {
		t.Fatal("logs not created", err)
	}
}

func TestOverridesAreRelativeToExecutableNotWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	for _, override := range []string{"portable-data", filepath.Join(t.TempDir(), "external-data")} {
		p, err := desktop.PathsFromExecutable(filepath.Join(root, "CAF.exe"), override)
		want := override
		if !filepath.IsAbs(want) {
			want = filepath.Join(root, want)
		}
		if err != nil || p.Data != want {
			t.Fatalf("unexpected override: %+v, %v", p, err)
		}
	}
}

func TestPathsRejectRelativeFields(t *testing.T) {
	if _, err := desktop.PathsFromExecutable("CAF.exe", ""); err == nil {
		t.Fatal("relative executable accepted")
	}
	valid := testPaths(t)
	for _, p := range []platform.Paths{
		{Root: "relative", Data: valid.Data, Logs: valid.Logs},
		{Root: valid.Root, Data: "relative", Logs: valid.Logs},
		{Root: valid.Root, Data: valid.Data, Logs: "relative"},
	} {
		if err := p.Validate(); err == nil {
			t.Fatal("relative path accepted", p)
		}
	}
}

func TestDataDirectoryCannotBeAFile(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Data, []byte("keep intact"), 0600); err != nil {
		t.Fatal(err)
	}
	b := desktop.New()
	if err := b.Init(p); err == nil {
		t.Fatal("file accepted as data directory")
	}
	if content, err := os.ReadFile(p.Data); err != nil || string(content) != "keep intact" {
		t.Fatal("file modified", err)
	}
}
