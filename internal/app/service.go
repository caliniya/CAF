// Package app contains shared application services. It does not import Wails or goinvoke.
package app

import (
	"errors"
	"runtime"
	"sync"

	"github.com/caliniya/CAF-Go/internal/lagrange"
	"github.com/caliniya/CAF-Go/internal/paths"
)

const Version = "0.1.0"

type NativeLoader interface {
	Status() lagrange.Status
	Load(dataDirectory string) (lagrange.Status, error)
}

type Status struct {
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Platform  string          `json:"platform"`
	Ready     bool            `json:"ready"`
	DataDir   string          `json:"dataDir"`
	Native    lagrange.Status `json:"native"`
	LastError string          `json:"lastError,omitempty"`
}

type Service struct {
	mu        sync.Mutex
	paths     paths.Paths
	native    NativeLoader
	lastError string
}

func New(directories paths.Paths, native NativeLoader) (*Service, error) {
	if native == nil {
		return nil, errors.New("a native loader is required")
	}
	if err := directories.Ensure(); err != nil {
		return nil, err
	}
	return &Service{paths: directories, native: native}, nil
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status()
}

func (s *Service) status() Status {
	return Status{
		Name: "CAF", Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Ready: true, DataDir: s.paths.Data, Native: s.native.Status(), LastError: s.lastError,
	}
}

func (s *Service) LoadNative() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.native.Load(s.paths.Data)
	s.lastError = ""
	if err != nil {
		s.lastError = err.Error()
	}
	return s.status(), err
}
