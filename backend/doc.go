// Package backend 是具体平台后端的组织入口，各实现放在 desktop 等子包。
// 后端实现 platform 契约；由程序入口注入 core，不另设桌面 App 业务转发层。
// 原生库属于后端实现细节，当前只保留桌面端资源和代码，不加载 QQ 库。
package backend
