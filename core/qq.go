package core

import (
	"errors"

	"github.com/caliniya/caf/core/platform"
)

// CancelLogin 取消二维码登录或关闭当前 QQ 会话，不关闭整个应用。
func (c *Core) CancelLogin() (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.readyError()
	if err == nil {
		if control, ok := c.qq.(platform.QQLoginControl); ok {
			err = control.CancelLogin()
		} else {
			err = platform.ErrQQUnavailable
		}
	}
	c.record(err)
	return c.statusLocked(), err
}

// SendGroupText 只在调用方显式请求时发送，不在登录后自动发测试消息。
func (c *Core) SendGroupText(group uint32, text string) (Status, error) {
	c.mu.Lock()
	err := c.readyError()
	messenger, ok := c.qq.(platform.QQMessenger)
	ctx := c.ctx
	if err == nil && (!ok || ctx == nil) {
		err = platform.ErrQQUnavailable
	}
	c.mu.Unlock()
	// 网络调用不持有核心锁，状态查询和取消仍可进行。
	if err == nil {
		err = messenger.SendGroupText(ctx, group, text)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return c.statusLocked(), errors.Join(err, platform.ErrClosed)
	}
	c.record(err)
	return c.statusLocked(), err
}
