//go:build windows

package builtin

import "testing"

// TestProcExecKeepsBackgroundProcess 验证 cmd 正常结束后，start /b 启动的
// 后台程序还在运行。
func TestProcExecKeepsBackgroundProcess(t *testing.T) {
	runBackgroundKept(t, map[string]any{
		"command": "start /b ping -n 20 127.0.0.1 > nul & echo started",
		"shell":   "cmd",
	}, "ping")
}

// TestProcExecTimeoutKillsTreeWindows 验证超时后整棵进程树被结束。
func TestProcExecTimeoutKillsTreeWindows(t *testing.T) {
	runTimeoutKillsChildren(t, "start /b ping -n 30 127.0.0.1 > nul & ping -n 30 127.0.0.1 > nul", "cmd", "ping")
}
