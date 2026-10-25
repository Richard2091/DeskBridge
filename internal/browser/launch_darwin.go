//go:build darwin

package browser

// macOS：每个浏览器实例一个进程组，关闭会话时只结束这个进程组。

import (
	"os/exec"
	"syscall"
)

// processGroup 在 macOS 上是进程组。
type processGroup struct{}

// prepareCmd 设置启动属性。
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// newProcessGroup 无需额外准备。
func newProcessGroup() (*processGroup, error) { return &processGroup{}, nil }

// attach 在 macOS 上不需要额外处理。
func (g *processGroup) attach(pid int) {}

// terminate 结束这个进程组。
func (g *processGroup) terminate(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	terminateProcess(cmd)
}

// release 无需额外处理。
func (g *processGroup) release() {}

// terminateProcess 结束根进程。
func terminateProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
