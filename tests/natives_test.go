package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caliniya/caf/backend/desktop/natives"
)

func TestBundledDllIsARealWindowsBinary(t *testing.T) {
	path, err := natives.Extract(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	content := readCachedNative(t, path)
	if len(content) < 2 || !bytes.Equal(content[:2], []byte("MZ")) {
		t.Fatal("expected a real PE binary, not an LFS pointer or placeholder")
	}
	provenance := nativeProvenance(t)
	if int64(len(content)) != provenance.Bytes {
		t.Fatal("bundled DLL size does not match PROVENANCE.json")
	}
	want := filepath.Join("natives", "win-x64", provenance.SHA256, "Lagrange.Core.NativeAPI.dll")
	if !strings.HasSuffix(path, want) {
		t.Fatal("cache path should be content-addressed", path)
	}
}

type nativeMetadata struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func nativeProvenance(t *testing.T) nativeMetadata {
	t.Helper()
	// go test 在 tests/ 下运行；读取受版本管理的来源记录，不访问资源包内部变量。
	metadata, err := os.ReadFile(filepath.Join("..", "backend", "desktop", "natives", "PROVENANCE.json"))
	if err != nil {
		t.Fatal(err)
	}
	var provenance nativeMetadata
	if err := json.Unmarshal(metadata, &provenance); err != nil {
		t.Fatal(err)
	}
	if _, err := hex.DecodeString(provenance.SHA256); err != nil || len(provenance.SHA256) != 64 || provenance.Bytes <= 0 {
		t.Fatal("invalid native provenance", provenance, err)
	}
	return provenance
}

func readCachedNative(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(content)
	if hex.EncodeToString(hash[:]) != nativeProvenance(t).SHA256 {
		t.Fatal("extracted DLL does not match PROVENANCE.json", path)
	}
	return content
}

func TestExtractReusesAndRepairsCache(t *testing.T) {
	data := filepath.Join(t.TempDir(), "便携目录 with spaces")
	path, err := natives.Extract(data)
	if err != nil {
		t.Fatalf("extract: %s, %v", path, err)
	}
	readCachedNative(t, path)
	old := time.Unix(1000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	again, err := natives.Extract(data)
	if err != nil || again != path {
		t.Fatal("cache path changed", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(old) {
		t.Fatal("unchanged cache should not be rewritten", err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = natives.Extract(data); err != nil {
		t.Fatal("corrupt cache was not repaired", err)
	}
	readCachedNative(t, path)
	temporary, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatal("temporary files leaked", temporary)
	}
}

func TestConcurrentExtraction(t *testing.T) {
	data := t.TempDir()
	var wait sync.WaitGroup
	results := make(chan string, 4)
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			path, err := natives.Extract(data)
			if err != nil {
				t.Error(err)
				return
			}
			results <- path
		}()
	}
	wait.Wait()
	close(results)
	want := ""
	for path := range results {
		if want == "" {
			want = path
		}
		if path != want {
			t.Fatal("inconsistent cache", path)
		}
		readCachedNative(t, path)
	}
	if want == "" {
		t.Fatal("no native cache was extracted")
	}
}

func TestRejectsInvalidDataPaths(t *testing.T) {
	if _, err := natives.Extract("relative-data"); err == nil {
		t.Fatal("relative cache accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("keep intact"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := natives.Extract(file); err == nil {
		t.Fatal("file accepted as cache directory")
	}
}
