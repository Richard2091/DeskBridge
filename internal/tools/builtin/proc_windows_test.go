//go:build windows

package builtin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/tools"
	"golang.org/x/sys/windows"
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

// TestConsoleCreationFlags 验证默认不弹控制台窗口、可见模式新建窗口。
func TestConsoleCreationFlags(t *testing.T) {
	if got := consoleCreationFlags(false); got != windows.CREATE_NO_WINDOW {
		t.Errorf("hidden 模式的创建标志 = %#x，期望 CREATE_NO_WINDOW", got)
	}
	if got := consoleCreationFlags(true); got != windows.CREATE_NEW_CONSOLE {
		t.Errorf("visible 模式的创建标志 = %#x，期望 CREATE_NEW_CONSOLE", got)
	}
}

// TestVisibleCommand 验证可见模式把命令交给 console 子命令，标题和回显都在参数里。
func TestVisibleCommand(t *testing.T) {
	oldExe, oldHold := helperExecutable, consoleHold
	helperExecutable = func() (string, error) { return `C:\fake\deskbridge.exe`, nil }
	consoleHold = 3 * time.Second
	defer func() { helperExecutable, consoleHold = oldExe, oldHold }()

	shellCmd, err := buildShellCommand("cmd", "echo hi")
	if err != nil {
		t.Fatalf("构造命令失败: %v", err)
	}
	helper, err := visibleCommand(shellCmd, "echo hi", `C:\work> `)
	if err != nil {
		t.Fatalf("包装可见命令失败: %v", err)
	}
	if len(helper.Args) < 2 || helper.Args[1] != "console" {
		t.Fatalf("helper 参数 = %q，期望第二个是 console", helper.Args)
	}
	joined := strings.Join(helper.Args, "\n")
	for _, want := range []string{"-title", "DeskBridge", "-line", `C:\work> echo hi`, "-cmdline", "-hold", "3s"} {
		if !strings.Contains(joined, want) {
			t.Errorf("helper 参数缺少 %q：%q", want, helper.Args)
		}
	}
}

// TestProcExecVisibleWindows 验证 visible 模式的完整链路：在设备桌面上新建
// 控制台窗口运行命令，返回退出码，不返回输出。需要先把本仓库编译出的
// deskbridge.exe 路径放进环境变量 DESKBRIDGE_HELPER_EXE（测试程序的
// os.Executable 不是 deskbridge，不能当 console 子命令用）。
func TestProcExecVisibleWindows(t *testing.T) {
	exe := os.Getenv("DESKBRIDGE_HELPER_EXE")
	if exe == "" {
		t.Skip("未设置 DESKBRIDGE_HELPER_EXE，跳过可见窗口的集成测试")
	}
	oldExe, oldHold := helperExecutable, consoleHold
	helperExecutable = func() (string, error) { return exe, nil }
	consoleHold = 0
	defer func() { helperExecutable, consoleHold = oldExe, oldHold }()

	res, err := callExec(t, map[string]any{
		"command": "exit /b 7",
		"shell":   "cmd",
		"console": "visible",
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("visible 模式执行失败: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("exit_code = %d，期望 7", res.ExitCode)
	}
	if res.Stdout != "" || res.Stderr != "" {
		t.Errorf("visible 模式不应返回输出：stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
	if !res.Console {
		t.Error("结果里的 console 应为 true")
	}
}
