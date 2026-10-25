//go:build darwin

package browser

// macOS：新进程组，结束进程时整组一起杀。父进程退出时没有 Pdeathsig，
// 浏览器可能残留，后续可以考虑注册退出处理。

import (
	"os/exec"
	"syscall"
)

// prepareCmd 设置启动属性。
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// assignToJob 在 macOS 上不需要额外处理。
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
