//go:build !windows

package platform

// HideConsole 只在 Windows 上有意义（其他平台没有随进程弹出的控制台窗口）。
func HideConsole() error { return ErrUnsupportedPlatform }
