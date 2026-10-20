package platform

import (
	"fmt"
	"path/filepath"
)

// Paths contains resolved paths. Platforms choose defaults; core only stores a copy.
type Paths struct {
	Root string `json:"root"`
	Data string `json:"data"`
	Logs string `json:"logs"`
}

// Validate does not read or create files. Filesystem preparation belongs to the backend.
func (p Paths) Validate() error {
	for _, item := range []struct{ name, path string }{
		{"root", p.Root}, {"data", p.Data}, {"logs", p.Logs},
	} {
		if !filepath.IsAbs(item.path) {
			return fmt.Errorf("%s path must be absolute", item.name)
		}
	}
	return nil
}
