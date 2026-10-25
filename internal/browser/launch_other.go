//go:build !windows && !linux && !darwin

package browser

// 其他平台：不支持浏览器工具。

import "os/exec"

func prepareCmd(cmd *exec.Cmd) {}

func assignToJob(pid int) {}

func terminate(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
