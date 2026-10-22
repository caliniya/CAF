package core

import "errors"

// StartLogin 是前端入口，实际登录交给平台实现。
func (c *Core) StartLogin() (int, error) {
	if c == nil || c.QQPlatform == nil {
		return 0, errors.New("QQ 平台尚未初始化")
	}

	return c.QQPlatform.StartLogin(), nil
}
