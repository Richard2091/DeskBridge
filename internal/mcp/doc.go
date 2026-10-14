// Package mcp 实现 MCP 服务器：把设备和工具以一组固定的通用工具
// 暴露给 AI Agent。它经本机接口（internal/client）调用本机节点，自己
// 不加入内网。标准输入输出被协议占用，日志只能写标准错误。
package mcp
