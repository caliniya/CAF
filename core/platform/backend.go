// Package platform defines platform contracts and plain Go data, not implementations.
package platform

import (
	"errors"
)

var (
	ErrBackendRequired     = errors.New("a platform backend is required")
	ErrNotInitialized      = errors.New("platform runtime has not been initialized")
	ErrAlreadyInitialized  = errors.New("platform paths cannot change after initialization")
	ErrClosed              = errors.New("platform runtime is closed")
	ErrContextRequired     = errors.New("a context is required")
	ErrUnsupportedPlatform = errors.New("native loading is not implemented on this platform")
	ErrLoginNotImplemented = errors.New("QQ login is not implemented; native loading only resolves exports")
)

type NativeStatus struct {
	Supported bool   `json:"supported"`
	Loaded    bool   `json:"loaded"`
	Path      string `json:"path,omitempty"`
	Exports   int    `json:"exports"`
}

type Backend interface {
	Init(Paths) error
	NativeStatus() NativeStatus
	LoadNative() (NativeStatus, error)
	Close() error
}                                                        
