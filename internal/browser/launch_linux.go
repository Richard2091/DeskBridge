//go:build linux

package browser

// Linux：新进程组，父进程退出时子进程收到 SIGKILL（Pdeathsig），
// 结束进程时整组一起杀。

import (
	"os/exec"
	"syscall"
)

// prepareCmd 设置启动属性。
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}

// assignToJob 在 Linux 上不需要额外处理。
func assignToJob(pid int) {}

// terminate 结束整个进程组。
func terminate(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
