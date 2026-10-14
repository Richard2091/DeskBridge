//go:build linux

package builtin

import (
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// sysPlatform 取 sys.info 里 Linux 特有的字段。
func sysPlatform() sysPlatformInfo {
	var p sysPlatformInfo
	if u, err := syscall.Uname(); err == nil {
		p.OSVersion = int8String(u.Release[:])
	}
	if cu, err := user.Current(); err == nil {
		p.User = cu.Username
		p.Home = cu.HomeDir
	}
	if total, avail, ok := readMemInfo(); ok {
		p.MemTotal, p.MemAvail = total, avail
	}
	if bt, ok := readBootTime(); ok {
		p.BootTime = bt
	}
	// 规范：interactive 表示有 DISPLAY 或 WAYLAND_DISPLAY；id 取不到时省略。
	p.Session = &sessionInfo{Interactive: os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""}
	return p
}

// readMemInfo 从 /proc/meminfo 读内存总量和可用量。
func readMemInfo() (total, avail int64, ok bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			avail = kb * 1024
		}
	}
	return total, avail, total > 0
}

// readBootTime 从 /proc/stat 的 btime 行读开机时间。
func readBootTime() (time.Time, bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "btime ") {
			sec, err := strconv.ParseInt(strings.TrimSpace(line[6:]), 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			return time.Unix(sec, 0), true
		}
	}
	return time.Time{}, false
}

// int8String 把 Uname 里的 int8 数组转成到第一个 0 为止的字符串。
func int8String(s []int8) string {
	b := make([]byte, 0, len(s))
	for _, c := range s {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}
