// Package app provides application services over the injected core runtime.
package app

import (
	"context"
	"errors"
	"runtime"

	"github.com/caliniya/caf/core"
	"github.com/caliniya/caf/core/platform"
)

const Version = "0.1.0"

type Status struct {
	Name      string                `json:"name"`
	Version   string                `json:"version"`
	Platform  string                `json:"platform"`
	Ready     bool                  `json:"ready"`
	Closed    bool                  `json:"closed"`
	DataDir   string                `json:"dataDir"`
	Native    platform.NativeStatus `json:"native"`
	LastError string                `json:"lastError,omitempty"`
}

type Service struct{ runtime *core.Runtime }

func New(rt *core.Runtime) (*Service, error) {
	if rt == nil {
		return nil, errors.New("a core runtime is required")
	}
	return &Service{runtime: rt}, nil
}

func (s *Service) Init() error { return s.runtime.Init() }

func (s *Service) Status() Status {
	state := s.runtime.State()
	return Status{
		Name: "CAF", Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Ready: state.Ready, Closed: state.Closed, DataDir: s.runtime.Paths().Data,
		Native: state.Native, LastError: state.LastError,
	}
}

func (s *Service) LoadNative() (Status, error) {
	err := s.runtime.LoadNative()
	return s.Status(), err
}

func (s *Service) BeginLogin(ctx context.Context) (Status, error) {
	err := s.runtime.BeginLogin(ctx)
	return s.Status(), err
}

func (s *Service) Close() error { return s.runtime.Close() }
