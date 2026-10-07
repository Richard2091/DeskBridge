// Package platform 集中放置按平台区分的功能。平台相关的实现写在
// _windows.go、_linux.go、_darwin.go 中；没有实现的平台返回
// ErrUnsupportedPlatform，不能静默失败。
package platform

import "errors"

// ErrUnsupportedPlatform 表示当前平台没有实现该功能。返回给调用方时，
// 错误码为 "unsupported_platform"。
var ErrUnsupportedPlatform = errors.New("unsupported_platform")

// DesktopHelperName 返回桌面辅助进程的可执行文件名。辅助进程目前只有
// Windows 版，其他平台返回 ErrUnsupportedPlatform。
func DesktopHelperName() (string, error) {
	return desktopHelperName()
}
