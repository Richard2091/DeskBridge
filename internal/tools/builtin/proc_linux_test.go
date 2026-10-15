//go:build linux

package builtin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/tools"
)

// hasProcessNamed 检查 /proc 里是否还有指定名称（comm）的进程。
func hasProcessNamed(t *testing.T, name string) bool {
	t.Helper()
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatalf("读 /proc 失败: %v", err)
	}
	for _, d := range dirs {
		if _, err := strconv.Atoi(d.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + d.Name() + "/comm")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(b)) == name {
			return true
		}
	}
	return false
}

// TestProcExecTimeoutKillsTree 验证超时后整棵进程树都被结束：命令
// fork 一个后台 sleep 再等待，超时后 /proc 里不应该再有 sleep。
func TestProcExecTimeoutKillsTree(t *testing.T) {
	r := newTestRegistry(t)
	tool, _ := r.Get("proc.exec")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 后台 sleep 由 shell fork 出来，是被结束树的真正目标。
	v, err := tool.Run(ctx, &tools.Call{RequestID: "t", Args: []byte(`{"command":"sleep 30 & wait"}`)})
	if err != nil {
		t.Fatalf("超时应返回结果而不是错误: %v", err)
	}
	if res := v.(*execResult); !res.TimedOut {
		t.Fatalf("timed_out = false，期望 true（%+v）", res)
	}
	if !hasProcessNamed(t, "sleep") {
		t.Skip("没找到 sleep 进程，可能命令还没来得及 fork（不影响结论）")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !hasProcessNamed(t, "sleep") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("超时后 5 秒，子进程 sleep 还在运行，进程树没有被结束")
}

// TestProcExecKeepsBackgroundProcess 验证 sh 正常结束后，后台的 sleep 还在。
func TestProcExecKeepsBackgroundProcess(t *testing.T) {
	sleep, name := uniqueProgram(t, "sleep")
	runBackgroundKept(t, map[string]any{"command": "'" + sleep + "' 20 & echo started", "shell": "sh"}, name)
}

// TestProcExecTimeoutKillsChildrenLinux 验证超时返回 timed_out 且子进程被结束。
func TestProcExecTimeoutKillsChildrenLinux(t *testing.T) {
	sleep, name := uniqueProgram(t, "sleep")
	runTimeoutKillsChildren(t, "'"+sleep+"' 30 & '"+sleep+"' 30", "sh", name)
}

// startAndReap 启动一个立刻退出的进程并 Wait 回收，返回此时已不存在的 pid。
func startAndReap(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动进程失败: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("等待进程退出失败: %v", err)
	}
	return pid
}

// TestKillProcessesPartialGone 验证目标里部分已退出（ESRCH）、部分结束成功时
// 算成功，killed 只列实际结束的；全部不存在时返回 ESRCH。
func TestKillProcessesPartialGone(t *testing.T) {
	dead := startAndReap(t)

	live := exec.Command("sleep", "60")
	if err := live.Start(); err != nil {
		t.Fatalf("启动 sleep 失败: %v", err)
	}
	defer func() {
		// 兜底：测试失败时也不留下 sleep；错误可忽略（进程可能已被结束）。
		live.Process.Kill() //nolint:errcheck
		live.Wait()         //nolint:errcheck
	}()

	killed, err := killProcesses([]int{dead, live.Process.Pid}, true)
	if err != nil {
		t.Fatalf("部分成员已退出应算成功，得到错误: %v", err)
	}
	if len(killed) != 1 || killed[0] != live.Process.Pid {
		t.Fatalf("killed 应只含存活进程 %d，得到 %v", live.Process.Pid, killed)
	}
	if err := live.Wait(); err == nil {
		t.Fatal("sleep 应是被信号结束的")
	}

	killed, err = killProcesses([]int{dead}, true)
	if !errors.Is(err, syscall.ESRCH) || len(killed) != 0 {
		t.Fatalf("全部不存在应返回 ESRCH，得到 killed=%v err=%v", killed, err)
	}
}
