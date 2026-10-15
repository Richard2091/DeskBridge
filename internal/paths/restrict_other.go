//go:build !windows

package paths

import (
	"fmt"
	"os"
)

// RestrictDir 把目录权限设为 0700，只允许当前用户访问。
func RestrictDir(dir string) error {
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("设置 %s 的访问权限失败: %w", dir, err)
	}
	return nil
}

// RestrictFile 把文件权限设为 0600，只允许当前用户访问。
func RestrictFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("设置 %s 的访问权限失败: %w", path, err)
	}
	return nil
}
