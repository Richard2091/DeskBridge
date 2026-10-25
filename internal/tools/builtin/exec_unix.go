//go:build linux || darwin

package builtin

import (
	"os/exec"
	"syscall"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// defaultShell 是 proc.exec 未指定 shell 时使用的 shell。
const defaultShell = "sh"

// shellAllowed 返回允许的 shell。
func shellAllowed() []string { return []string{"sh", "bash"} }

// buildShellCommand 构造经 shell 执行的命令。
func buildShellCommand(shell, command string) (*exec.Cmd, error) {
	path, err := exec.LookPath(shell)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeNotFound, "找不到 %s：%v", shell, err)
	}
	return exec.Command(path, "-c", command), nil
}

// visibleCommand 只在 Windows 上实现：在设备桌面上打开带标题的控制台窗口。
func visibleCommand(shellCmd *exec.Cmd, display, prompt string) (*exec.Cmd, error) {
	return nil, protocol.Errorf(protocol.CodeUnsupportedPlatform, "可见控制台窗口只在 Windows 上支持")
}

// startProcess 启动进程并放进独立进程组（Setpgid），超时或取消时对
// 整个进程组发 SIGKILL，结束整棵进程树。
func startProcess(cmd *exec.Cmd, visible bool) (kill, cleanup func(), err error) {
	if visible {
		return nil, nil, protocol.Errorf(protocol.CodeUnsupportedPlatform, "可见控制台窗口只在 Windows 上支持")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if serr := cmd.Start(); serr != nil {
		return nil, nil, serr
	}
	pgid := cmd.Process.Pid
	kill = func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	cleanup = func() {}
	return kill, cleanup, nil
}
