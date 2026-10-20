//go:build darwin

package builtin

import (
	"golang.org/x/sys/unix"

	"github.com/Richard2091/DeskBridge/internal/tools"
)

// procTools 返回 macOS 上的 proc 工具：三个都有实现。
func procTools() []*tools.Tool {
	return []*tools.Tool{newProcExecTool(), newProcListTool(), newProcKillTool()}
}

// listProcesses 用 sysctl kern.proc.all 列出所有进程：PID、PPID、
// 用户（real uid）和名称。名称来自 p_comm，系统最多保存 16 字节，
// 较长的进程名会被截断。内存占用（memory_bytes）待实现，先省略。
func listProcesses() ([]procEntry, bool) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, false
	}
	procs := make([]procEntry, 0, len(kps))
	for i := range kps {
		kp := &kps[i]
		name := cString(kp.Proc.P_comm[:])
		if name == "" {
			continue
		}
		procs = append(procs, procEntry{
			PID:  int(kp.Proc.P_pid),
			PPID: int(kp.Eproc.Ppid),
			Name: name,
			User: userNameOf(int(kp.Eproc.Pcred.P_ruid)),
		})
	}
	return procs, true
}

// descendantsOf 用一次 sysctl 的结果构造 pid→ppid，找出 pid 的全部后代。
func descendantsOf(pid int) []int {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	parents := make(map[int]int, len(kps))
	for i := range kps {
		parents[int(kps[i].Proc.P_pid)] = int(kps[i].Eproc.Ppid)
	}
	return childrenOf(pid, parents)
}
