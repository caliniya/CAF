package core

import (
	"runtime"

	"github.com/caliniya/caf/core/platform"
)

const Version = "0.1.0"

// State 是核心生命周期与原生库状态的快照。
type State struct {
	Ready     bool                  `json:"ready"`
	Closed    bool                  `json:"closed"`
	Native    platform.NativeStatus `json:"native"`
	LastError string                `json:"lastError,omitempty"`
}

// Status 直接返回给界面，原先 Service 中的数据也统一放回 core。
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

func (c *Core) stateLocked() State {
	var native platform.NativeStatus
	if c.backend != nil {
		native = c.backend.NativeStatus()
	}
	return State{Ready: c.initialized && !c.closed, Closed: c.closed, Native: native, LastError: c.lastError}
}

// State 只查询状态，不初始化或加载平台资源。
func (c *Core) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stateLocked()
}

func (c *Core) statusLocked() Status {
	state := c.stateLocked()
	return Status{
		Name: "CAF", Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Ready: state.Ready, Closed: state.Closed, DataDir: c.paths.Data,
		Native: state.Native, LastError: state.LastError,
	}
}

// GetStatus 供前端和其他 Go 包查询，返回副本，不暴露后端或函数指针。
func (c *Core) GetStatus() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked()
}
