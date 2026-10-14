//go:build windows

package builtin

import (
	"os/exec"
	"strings"
	"testing"
)

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

// TestProcExecPowerShellPlainStderr 验证 PowerShell 的错误输出是纯文本，
// 不是 CLIXML，也没有进度记录；同时命令里的双引号和中文输出不受影响。
func TestProcExecPowerShellPlainStderr(t *testing.T) {
	shells := []string{"powershell"}
	if _, err := exec.LookPath("pwsh"); err == nil {
		shells = append(shells, "pwsh")
	}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			res, err := callExec(t, map[string]any{
				"command": `Get-Item C:\不存在目录; Write-Output "中文 输出"`,
				"shell":   shell,
			}, 0)
			if err != nil {
				t.Fatalf("proc.exec 失败: %v", err)
			}
			if strings.HasPrefix(res.Stderr, "#< CLIXML") || strings.Contains(res.Stderr, "<Objs") {
				t.Errorf("stderr 是 CLIXML：%q", res.Stderr)
			}
			if !strings.Contains(res.Stderr, `C:\不存在目录`) {
				t.Errorf("stderr 缺少中文路径：%q", res.Stderr)
			}
			if !strings.Contains(res.Stdout, "中文 输出") {
				t.Errorf("stdout 中文不对：%q", res.Stdout)
			}
		})
	}
}
