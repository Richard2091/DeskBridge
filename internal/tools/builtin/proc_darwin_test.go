//go:build darwin

package builtin

import (
	"os"
	"testing"
)

// 进程列表必须包含当前进程，且 PPID 与 os.Getppid 一致、名称非空。
func TestListProcessesDarwinSelf(t *testing.T) {
	procs, ok := listProcesses()
	if !ok {
		t.Fatal("listProcesses 失败")
	}
	pid := os.Getpid()
	for _, p := range procs {
		if p.PID != pid {
			continue
		}
		if p.PPID != os.Getppid() {
			t.Errorf("PPID = %d，期望 %d", p.PPID, os.Getppid())
		}
		if p.Name == "" {
			t.Error("名称为空")
		}
		return
	}
	t.Fatalf("列表中没有当前进程 %d", pid)
}
