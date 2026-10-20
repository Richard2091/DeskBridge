//go:build !windows && !darwin

package autostart

import "github.com/Richard2091/DeskBridge/internal/platform"

// Install 在 Windows 以外的平台没有实现。
func Install(Config) error { return platform.ErrUnsupportedPlatform }

// Uninstall 在 Windows 以外的平台没有实现。
func Uninstall(string) error { return platform.ErrUnsupportedPlatform }

// Start 在 Windows 以外的平台没有实现。
func Start(string) error { return platform.ErrUnsupportedPlatform }

// Exists 在 Windows 以外的平台恒为 false。
func Exists(string) bool { return false }
