//go:build !windows

package supervise

import "os/exec"

// Prepare 在 Windows 以外的平台不需要做什么。
func Prepare(*exec.Cmd) {}

// Attach 在 Windows 以外的平台不需要做什么（守护进程的部署方式是 systemd 等）。
func Attach(*exec.Cmd) error { return nil }
