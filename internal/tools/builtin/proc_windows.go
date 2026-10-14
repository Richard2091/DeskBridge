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

// descendantsOf 用进程快照找出 pid 的全部后代（不含 pid 自己）。
func descendantsOf(pid int) []int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	parents := make(map[int]int)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil
	}
	for {
		parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return childrenOf(pid, parents)
}

// killProcesses 结束给定 PID（Windows 上总是强制结束，force 只为兼容
// 参数）。返回实际结束的 PID 和最后一个失败。
func killProcesses(pids []int, force bool) ([]int, error) {
	var killed []int
	var lastErr error
	for _, pid := range pids {
		if err := terminateWindowsProcess(pid); err != nil {
			lastErr = err
			continue
		}
		killed = append(killed, pid)
	}
	return killed, lastErr
}

// terminateWindowsProcess 强制结束单个进程。
func terminateWindowsProcess(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
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
