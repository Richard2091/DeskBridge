//go:build windows

package builtin

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// MB_ERR_INVALID_CHARS 是 MultiByteToWideChar 的标志位，表示遇到
// 无效字节就报错；x/sys/windows 未定义，值来自 Win32 文档。
const mbErrInvalidChars = 0x8

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modpsapi    = windows.NewLazySystemDLL("psapi.dll")

	procGetOEMCP             = modkernel32.NewProc("GetOEMCP")
	procGetTickCount64       = modkernel32.NewProc("GetTickCount64")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetProcessMemoryInfo = modpsapi.NewProc("GetProcessMemoryInfo")
)

// getOEMCP 返回系统 OEM 代码页（cmd 控制台输出用的编码）。
func getOEMCP() uint32 {
	r1, _, _ := procGetOEMCP.Call()
	return uint32(r1)
}

// getTickCount64 返回开机以来经过的毫秒数。
func getTickCount64() uint64 {
	r1, _, _ := procGetTickCount64.Call()
	return uint64(r1)
}

// memoryStatusEx 对应 Win32 的 MEMORYSTATUSEX，字段顺序和类型照 MSDN
// （DWORD 用 uint32，DWORDLONG 用 uint64）。
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// globalMemoryStatusEx 查询系统物理内存和虚拟内存的总量与可用量。
func globalMemoryStatusEx(ms *memoryStatusEx) error {
	r1, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(ms)))
	if r1 == 0 {
		if err != nil {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

// processMemoryCounters 对应 Win32 的 PROCESS_MEMORY_COUNTERS，SIZE_T
// 用 uintptr（与 x/sys/windows 自带结构体的做法一致）。
type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// getProcessMemoryInfo 查询进程的内存使用情况（psapi.dll）。
func getProcessMemoryInfo(h windows.Handle, pmc *processMemoryCounters) error {
	r1, _, err := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(pmc)), uintptr(pmc.Cb))
	if r1 == 0 {
		if err != nil {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
