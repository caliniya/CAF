// Package client 预留 HTTP/WebSocket 通信、连接管理、超时与重连。
// 当前只建立包结构，不监听端口、不连接 API，也不自动启动反向 WebSocket。
// 协议解析交给 pre，业务决策和工具执行交给 core。
package client
