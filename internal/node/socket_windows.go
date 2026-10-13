//go:build windows

package node

// chmodSocket 在 Windows 上不需要设置 socket 文件权限：AF_UNIX 命名管道
// 的访问控制由用户会话决定。
func chmodSocket(string) error { return nil }
