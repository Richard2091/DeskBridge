//go:build !windows

package node

import "os"

// chmodSocket 把 socket 文件权限设为 0600，只允许同一用户连接。
func chmodSocket(path string) error {
	return os.Chmod(path, 0o600)
}
