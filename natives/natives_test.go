package natives

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBundledDllIsARealWindowsBinary(t *testing.T) {
	if len(binary) < 2 || !bytes.Equal(binary[:2], []byte("MZ")) {
		t.Fatal("expected a real PE binary, not an LFS pointer or placeholder")
	}
	hash := sha256.Sum256(binary)
	metadata, err := os.ReadFile("PROVENANCE.json")
	if err != nil {
		t.Fatal(err)
	}
	var provenance struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(metadata, &provenance); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(hash[:]) != provenance.SHA256 {
		t.Fatal("bundled DLL changed without updating PROVENANCE.json")
	}
}

func TestExtractReusesAndRepairsCache(t *testing.T) {
	data := filepath.Join(t.TempDir(), "便携目录 with spaces")
	path, err := Extract(data)
	if err != nil || !matches(path) {
		t.Fatalf("extract: %s, %v", path, err)
	}
	old := time.Unix(1000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	again, err := Extract(data)
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
	if _, err = Extract(data); err != nil || !matches(path) {
		t.Fatal("corrupt cache was not repaired", err)
	}
	temporary, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
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
			path, err := Extract(data)
			if err != nil {
				t.Error(err)
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
		if path != want || !matches(path) {
			t.Fatal("inconsistent cache", path)
		}
	}
}

func TestRejectsInvalidDataPaths(t *testing.T) {
	if _, err := Extract("relative-data"); err == nil {
		t.Fatal("relative cache accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("keep intact"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(file); err == nil {
		t.Fatal("file accepted as cache directory")
	}
}
