//go:build windows

package builtin

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/tools"
)

// TestProcExecKeepsBackgroundProcess 验证 cmd 正常结束后，start /b 启动的
// 后台程序还在运行。
func TestProcExecKeepsBackgroundProcess(t *testing.T) {
	ping, name := uniqueProgram(t, "ping")
	runBackgroundKept(t, map[string]any{
		"command": `start "" /b "` + ping + `" -n 20 127.0.0.1 > nul & echo started`,
		"shell":   "cmd",
	}, name)
}

// TestProcExecTimeoutKillsTreeWindows 验证超时后整棵进程树被结束。
func TestProcExecTimeoutKillsTreeWindows(t *testing.T) {
	ping, name := uniqueProgram(t, "ping")
	runTimeoutKillsChildren(t, `start "" /b "`+ping+`" -n 30 127.0.0.1 > nul & "`+ping+`" -n 30 127.0.0.1 > nul`, "cmd", name)
}

// TestProcKillTreeWindows 验证 tree=true 时子进程和孙进程都被结束，
// 返回的 killed 最后一个是根进程。
func TestProcKillTreeWindows(t *testing.T) {
	ping, name := uniqueProgram(t, "ping")
	before := listPIDsByName(t, name)
	// cmd 是根；start /b 启动的 ping 和前台的 ping 都是它的子进程。
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /c "start "" /b "` + ping +
		`" -n 30 127.0.0.1 > nul & "` + ping + `" -n 30 127.0.0.1 > nul"`}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试进程失败: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var children []int
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if children = newPIDs(before, listPIDsByName(t, name)); len(children) == 2 {
			break
		}
	}
	defer killPIDs(t, children)
	if len(children) != 2 {
		t.Fatalf("子进程 = %v，期望 2 个", children)
	}

	tool, _ := newTestRegistry(t).Get("proc.kill")
	raw, _ := json.Marshal(map[string]any{"pid": cmd.Process.Pid, "tree": true})
	v, err := tool.Run(context.Background(), &tools.Call{Args: raw})
	if err != nil {
		t.Fatalf("proc.kill 失败: %v", err)
	}
	killed := v.(*procKillResult).Killed
	if len(killed) != 3 || killed[2] != cmd.Process.Pid {
		t.Errorf("killed = %v，期望两个子进程加最后的根 %d", killed, cmd.Process.Pid)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("根进程在 proc.kill 后 5 秒还没有退出")
	}
	left := newPIDs(before, listPIDsByName(t, name))
	for deadline := time.Now().Add(5 * time.Second); len(left) > 0 && time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		left = newPIDs(before, listPIDsByName(t, name))
	}
	if len(left) > 0 {
		t.Errorf("子进程 %v 仍在运行", left)
	}
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
