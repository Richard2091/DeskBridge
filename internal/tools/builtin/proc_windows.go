//go:build windows

package builtin

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
	"golang.org/x/sys/windows"
)

// procTools 返回 Windows 上的 proc 工具：三个都有实现。
func procTools() []*tools.Tool {
	return []*tools.Tool{newProcExecTool(), newProcListTool(), newProcKillTool()}
}

// listProcesses 用 Toolhelp32 快照列出所有进程。内存常驻取不到时省略；
// Windows 上不提供 user 字段（取它需要打开每个进程的令牌，代价大）。
func listProcesses() ([]procEntry, bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, false
	}
	var procs []procEntry
	for {
		e := procEntry{
			PID:  int(entry.ProcessID),
			PPID: int(entry.ParentProcessID),
			Name: windows.UTF16ToString(entry.ExeFile[:]),
		}
		if h, oerr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID); oerr == nil {
			var pmc processMemoryCounters
			pmc.Cb = uint32(unsafe.Sizeof(pmc))
			if gerr := getProcessMemoryInfo(h, &pmc); gerr == nil {
				e.MemoryBytes = int64(pmc.WorkingSetSize)
			}
			windows.CloseHandle(h)
		}
		procs = append(procs, e)
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return procs, true
}

// snapshotParents 用 Toolhelp32 快照取得 pid→ppid 映射。
func snapshotParents() (map[int]int, bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	defer windows.CloseHandle(snap)
	parents := make(map[int]int)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, false
	}
	for {
		parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return parents, true
}

// descendantsOf 用进程快照找出 pid 的全部后代（不含 pid 自己）。
func descendantsOf(pid int) []int {
	parents, ok := snapshotParents()
	if !ok {
		return nil
	}
	// 快照的 PPID 可能过期（父进程退出后不更新，PID 会复用），用创建时间
	// 过滤。先按原始关系算出候选后代，只对根进程和候选（它们的父进程
	// 也在这个集合内）取创建时间，不必打开全系统的每个进程。
	// 取不到创建时间的进程不纳入。
	candidates := childrenOf(pid, parents)
	created := make(map[int]uint64, len(candidates)+1)
	for _, p := range append([]int{pid}, candidates...) {
		if t, ok := processCreationTime(uint32(p)); ok {
			created[p] = t
		}
	}
	return childrenOf(pid, plausibleParents(parents, created))
}

// processCreationTime 返回进程创建时间（FILETIME 的 64 位值）。
// 打不开进程或取不到时间时 ok 为 false。
func processCreationTime(pid uint32) (uint64, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	return handleCreationTime(h)
}

// handleCreationTime 用已打开的进程句柄读创建时间。
func handleCreationTime(h windows.Handle) (uint64, bool) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, false
	}
	return uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime), true
}

// killProcesses 结束给定 PID（Windows 上总是强制结束，force 只为兼容
// 参数）。返回实际结束的 PID 和最后一个失败。
//
// pids 的约定与 runProcKill 一致：最后一个是用户指定的目标，前面的是它
// 的后代（由深到浅）。每个进程只打开一次句柄并一直持有到结束之后：
// 句柄存活期间 PID 不会被复用，所以核对和结束之间没有空档。核对规则：
// 后代的快照父进程必须也在 pids 中、已成功打开，且后代创建时间不早于
// 父进程；打不开或核对失败的后代不结束。
func killProcesses(pids []int, force bool) ([]int, error) {
	type target struct {
		pid     int
		h       windows.Handle
		created uint64
		hasTime bool
	}
	var lastErr error
	targets := make(map[int]*target, len(pids))
	defer func() {
		for _, t := range targets {
			windows.CloseHandle(t.h)
		}
	}()
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			lastErr = err
			continue
		}
		t := &target{pid: pid, h: h}
		t.created, t.hasTime = handleCreationTime(h)
		targets[pid] = t
	}
	var parents map[int]int
	if len(pids) > 1 {
		var ok bool
		if parents, ok = snapshotParents(); !ok {
			// 无法核对父子关系时只处理用户指定的目标。
			lastErr = errors.New("无法取得进程快照，未结束子进程")
			parents = nil
		}
	}
	var killed []int
	for i, pid := range pids {
		t, ok := targets[pid]
		if !ok {
			continue
		}
		if i < len(pids)-1 {
			parent, pok := targets[parents[pid]]
			if !pok || !t.hasTime || !parent.hasTime || t.created < parent.created {
				lastErr = fmt.Errorf("进程 %d 与父进程的关系核对失败，未结束", pid)
				continue
			}
		}
		if err := windows.TerminateProcess(t.h, 1); err != nil {
			lastErr = err
			continue
		}
		killed = append(killed, pid)
	}
	return killed, lastErr
}

// mapKillError 把结束进程的失败映射为协议错误。
func mapKillError(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		return protocol.Errorf(protocol.CodeNotFound, "进程不存在：%v", err)
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return &protocol.Error{
			Code:    protocol.CodePermissionDenied,
			Message: fmt.Sprintf("操作系统拒绝了结束进程：%v", err),
			Details: map[string]any{"missing": "os_permission"},
		}
	default:
		return protocol.Errorf(protocol.CodeInternal, "结束进程失败：%v", err)
	}
}
