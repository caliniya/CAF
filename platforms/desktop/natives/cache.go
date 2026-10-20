// Package natives embeds and extracts the user's existing Windows x64 NativeAPI binary.
package natives

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed win-x64/Lagrange.Core.NativeAPI.dll
var binary []byte

const fileName = "Lagrange.Core.NativeAPI.dll"

// Extract uses a content-addressed path so upgrading never replaces a loaded DLL.
func Extract(dataDirectory string) (string, error) {
	if !filepath.IsAbs(dataDirectory) {
		return "", fmt.Errorf("native cache data directory must be absolute")
	}
	hash := sha256.Sum256(binary)
	directory := filepath.Join(dataDirectory, "natives", "win-x64", hex.EncodeToString(hash[:]))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(directory, fileName)
	if matches(target) {
		return target, nil
	}
	temporary, err := os.CreateTemp(directory, "lagrange-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(binary); err != nil {
		temporary.Close()
		return "", err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err = temporary.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(temporary.Name(), target); err != nil {
		// Another process may have installed and loaded the same DLL in the meantime.
		if !matches(target) {
			return "", fmt.Errorf("install native cache (a running process may lock it): %w", err)
		}
	}
	return target, nil
}

func matches(path string) bool {
	content, err := os.ReadFile(path)
	return err == nil && bytes.Equal(content, binary)
}
