// Package paths defines portable paths without depending on the launcher's working directory.
package paths

import (
	"errors"
	"os"
	"path/filepath"
)

type Paths struct {
	Home string
	Data string
	Logs string
}

func Discover(dataOverride string) (Paths, error) {
	executable, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	return FromExecutable(executable, dataOverride)
}

// Relative overrides are interpreted relative to the executable, never the current directory.
func FromExecutable(executable, dataOverride string) (Paths, error) {
	if !filepath.IsAbs(executable) {
		return Paths{}, errors.New("an absolute executable path is required")
	}
	home := filepath.Dir(filepath.Clean(executable))
	data := dataOverride
	if data == "" {
		data = "data"
	}
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, data)
	}
	data = filepath.Clean(data)
	return Paths{Home: home, Data: data, Logs: filepath.Join(data, "logs")}, nil
}

func (p Paths) Ensure() error {
	if !filepath.IsAbs(p.Data) || !filepath.IsAbs(p.Logs) {
		return errors.New("application data paths must be absolute")
	}
	return os.MkdirAll(p.Logs, 0700)
}
