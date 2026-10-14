//go:build linux

package builtin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// procTools 返回 Linux 上的 proc 工具：三个都有实现。
func procTools() []*tools.Tool {
	return []*tools.Tool{newProcExecTool(), newProcListTool(), newProcKillTool()}
}

// userNames 缓存 uid→用户名，避免每个进程都查一遍 /etc/passwd。
var (
	userNamesMu sync.Mutex
	userNames   = map[int]string{}
)

// listProcesses 读 /proc 列出所有进程：名称、PPid、Uid 取自 status，
// 常驻内存取 VmRSS。读不了的进程（刚好退出或无权限）跳过。
func listProcesses() ([]procEntry, bool) {
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	var procs []procEntry
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue // 不是进程目录
		}
		b, err := os.ReadFile("/proc/" + d.Name() + "/status")
		if err != nil {
			continue // 进程刚好退出
		}
		e, ok := parseStatus(pid, b)
		if !ok {
			continue
		}
		procs = append(procs, e)
	}
	return procs, true
}

// parseStatus 解析一个 /proc/<pid>/status 文件。
func parseStatus(pid int, b []byte) (procEntry, bool) {
	e := procEntry{PID: pid}
	for _, line := range strings.Split(string(b), "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch name {
		case "Name":
			e.Name = value
		case "PPid":
			if pp, err := strconv.Atoi(value); err == nil {
				e.PPID = pp
			}
		case "Uid":
			// real effective saved fs，只取 real。
			if fields := strings.Fields(value); len(fields) > 0 {
				if uid, err := strconv.Atoi(fields[0]); err == nil {
					e.User = userNameOf(uid)
				}
			}
		case "VmRSS":
			// 如 "1234 kB"。
			if fields := strings.Fields(value); len(fields) > 0 {
				if kb, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
					e.MemoryBytes = kb * 1024
				}
			}
		}
	}
	if e.Name == "" {
		return e, false
	}
	return e, true
}

// userNameOf 查 uid 对应的用户名，查不到返回空（结果里省略）。
func userNameOf(uid int) string {
	userNamesMu.Lock()
	defer userNamesMu.Unlock()
	if name, ok := userNames[uid]; ok {
		return name
	}
	name := ""
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		name = u.Username
	}
	userNames[uid] = name
	return name
}

// descendantsOf 从 /proc 收集 pid→ppid，找出 pid 的全部后代。
func descendantsOf(pid int) []int {
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	parents := make(map[int]int, len(dirs))
	for _, d := range dirs {
		p, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + d.Name() + "/stat")
		if err != nil {
			continue
		}
		if pp, ok := ppidOfStat(b); ok {
			parents[p] = pp
		}
	}
	return childrenOf(pid, parents)
}

// ppidOfStat 从 /proc/<pid>/stat 里取 PPid。comm 可能含空格和括号，
// 所以从最后一个 ')' 之后解析。
func ppidOfStat(b []byte) (int, bool) {
	i := bytes.LastIndexByte(b, ')')
	if i < 0 || i+2 >= len(b) {
		return 0, false
	}
	fields := strings.Fields(string(b[i+2:])) // fields[0] 是 state
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

// killProcesses 给目标发信号：force 为 true 发 SIGKILL，否则 SIGTERM。
// 进程已经不在（ESRCH）不算失败，只是不在 killed 列表里。
func killProcesses(pids []int, force bool) ([]int, error) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	var killed []int
	var lastErr error
	for _, pid := range pids {
		err := syscall.Kill(pid, sig)
		switch {
		case err == nil:
			killed = append(killed, pid)
		case errors.Is(err, syscall.ESRCH):
			// 进程刚好自己结束了。
		default:
			lastErr = err
		}
	}
	return killed, lastErr
}

// mapKillError 把结束进程的失败映射为协议错误。
func mapKillError(err error) error {
	switch {
	case errors.Is(err, syscall.ESRCH):
		return protocol.Errorf(protocol.CodeNotFound, "进程不存在：%v", err)
	case errors.Is(err, syscall.EPERM):
		return &protocol.Error{
			Code:    protocol.CodePermissionDenied,
			Message: fmt.Sprintf("操作系统拒绝了结束进程：%v", err),
			Details: map[string]any{"missing": "os_permission"},
		}
	default:
		return protocol.Errorf(protocol.CodeInternal, "结束进程失败：%v", err)
	}
}
