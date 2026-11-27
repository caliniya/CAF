//go:build !windows

package Platform

import "path/filepath"

// PhysicalPath returns the canonical location of an existing file or directory.
func PhysicalPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
