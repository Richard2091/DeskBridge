// Package paths 集中管理 DeskBridge 的本地目录和文件路径。
package paths

import (
	"fmt"
	"os"
	"runtime"
)

// Home 返回 DeskBridge 的本地数据目录。
// 环境变量 DESKBRIDGE_HOME 优先；否则使用用户配置目录下的
// DeskBridge（Windows、macOS）或 deskbridge（Linux）。
func Home() (string, error) {
	if v := os.Getenv("DESKBRIDGE_HOME"); v != "" {
		return v, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("确定用户配置目录失败: %w", err)
	}
	name := "deskbridge"
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		name = "DeskBridge"
	}
	return configDir + string(os.PathSeparator) + name, nil
}

// TailscaleDir 返回 tsnet 状态目录（Home 下的 tailscale）。
func TailscaleDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return home + string(os.PathSeparator) + "tailscale", nil
}

// SocketPath 返回本机接口的 unix socket 路径（Home 下的 node.sock）。
func SocketPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return home + string(os.PathSeparator) + "node.sock", nil
}

// DeviceKeyFile 返回设备私钥的备用文件路径（Home 下的 device.key）。
func DeviceKeyFile() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return home + string(os.PathSeparator) + "device.key", nil
}

// EnsureHome 创建数据目录并限制为只有当前用户能访问（已存在时也收紧），
// 返回目录路径。数据目录里有登录状态、设备密钥和本机接口 socket，连上
// socket 就获得本机身份，所以先收紧目录，再在里面创建这些文件：新文件
// 一创建就继承受限的权限，不存在“先创建、后改权限”的空档。
// DESKBRIDGE_HOME 应指向 DeskBridge 专用的目录。
func EnsureHome() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	if err := Ensure(home); err != nil {
		return "", err
	}
	if err := RestrictDir(home); err != nil {
		return "", err
	}
	return home, nil
}

// Ensure 创建目录（含父目录），权限 0700。已存在时不报错。
func Ensure(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
	}
	return nil
}

// LogDir 返回日志目录（Home 下的 logs）。
func LogDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return home + string(os.PathSeparator) + "logs", nil
}

// NodeLogFile 返回节点日志文件路径（日志目录下的 node.log）。
func NodeLogFile() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return dir + string(os.PathSeparator) + "node.log", nil
}
