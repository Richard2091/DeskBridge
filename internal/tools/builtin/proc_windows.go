//go:build windows

package builtin

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
	"golang.org/x/sys/windows"
)

// procTools 返回 Windows 上的 proc 工具：三个都有实现。
func procTools() []*tools.Tool {
	return []*tools.Tool{newProcExecTool(), newProcListTool(), newProcKillTool()}
}

// walkProcessSnapshot 用 Toolhelp32 快照遍历所有进程，对每个条目调用 fn。
// 快照或首个条目取不到时返回 false。
func walkProcessSnapshot(fn func(*windows.ProcessEntry32)) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return false
	}
	for {
		fn(&entry)
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return true
}

// listProcesses 用 Toolhelp32 快照列出所有进程。内存常驻取不到时省略；
// Windows 上不提供 user 字段（取它需要打开每个进程的令牌，代价大）。
func listProcesses() ([]procEntry, bool) {
	var procs []procEntry
	ok := walkProcessSnapshot(func(entry *windows.ProcessEntry32) {
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
	})
	return procs, ok
}

// snapshotParents 用 Toolhelp32 快照取得 pid→ppid 映射。
func snapshotParents() (map[int]int, bool) {
	parents := make(map[int]int)
	ok := walkProcessSnapshot(func(entry *windows.ProcessEntry32) {
		parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
	})
	return parents, ok
}

// handleCreationTime 用已打开的进程句柄读创建时间（FILETIME 的 64 位值）。
func handleCreationTime(h windows.Handle) (uint64, bool) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, false
	}
	return uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime), true
}

// killProcessTree 结束 pid；tree 为 true 时连同后代一起结束（Windows 上
// 总是强制结束，force 只为兼容参数）。返回实际结束的 PID（后代由深到浅，
// 最后是根）。err 非 nil 表示根进程没有被结束；后代结束失败不算错误，
// 宁可漏杀也不误杀。
//
// 流程：先打开根并一直持有句柄，根打不开、读不到创建时间、取不到快照时
// 都不结束任何进程。之后只取一次快照，对候选后代各打开一次句柄并持有到
// 结束之后——句柄存活期间 PID 不会被复用，核对和结束之间没有空档。
// 核对用 childrenOf(root, plausibleParents(...))：广度优先只沿通过核对的
// 边（子进程创建时间不早于父进程）向下走，中间节点没通过核对（打不开、
// 读不到时间、创建时间早于父进程）时它的整棵子树都不会出现，所以核对是
// 传递的。节点自己不在 created 里，它的子树同样不会被结束，这是有意的。
//
// 残留的空档：用户看到根 PID 到调用 proc.kill 之间，根 PID 可能已被复用，
// 只凭 PID 无法分辨。节点内部从打开根句柄开始没有空档。
func killProcessTree(pid int, tree, force bool) ([]int, error) {
	access := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_TERMINATE)
	root, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil && !tree && errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// 单进程结束不需要查询权限，退回只要结束权限。
		root, err = windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	}
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	if !tree {
		if err := windows.TerminateProcess(root, 1); err != nil {
			return nil, err
		}
		return []int{pid}, nil
	}

	rootCreated, ok := handleCreationTime(root)
	if !ok {
		return nil, fmt.Errorf("无法读取进程 %d 的创建时间，未结束任何进程", pid)
	}
	parents, ok := snapshotParents()
	if !ok {
		return nil, errors.New("无法取得进程快照，未结束任何进程")
	}
	self := os.Getpid()
	created := map[int]uint64{pid: rootCreated}
	handles := make(map[int]windows.Handle)
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	for _, p := range childrenOf(pid, parents) {
		if p == self {
			continue
		}
		h, err := windows.OpenProcess(access, false, uint32(p))
		if err != nil {
			continue
		}
		t, ok := handleCreationTime(h)
		if !ok {
			windows.CloseHandle(h)
			continue
		}
		handles[p] = h
		created[p] = t
	}
	var killed []int
	for _, p := range childrenOf(pid, plausibleParents(parents, created)) {
		h, ok := handles[p]
		if !ok { // created 里只有根和已持有句柄的候选，防御性检查
			continue
		}
		if err := windows.TerminateProcess(h, 1); err == nil {
			killed = append(killed, p)
		}
	}
	if err := windows.TerminateProcess(root, 1); err != nil {
		return killed, err
	}
	return append(killed, pid), nil
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
