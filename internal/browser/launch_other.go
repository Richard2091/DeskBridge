//go:build !windows && !linux && !darwin

package browser

// 其他平台：不支持浏览器工具。

import "os/exec"

type processGroup struct{}

func prepareCmd(cmd *exec.Cmd) {}

func newProcessGroup() (*processGroup, error) { return &processGroup{}, nil }

func (g *processGroup) attach(pid int) {}

func (g *processGroup) terminate(cmd *exec.Cmd) { terminateProcess(cmd) }

func (g *processGroup) release() {}

func terminateProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
