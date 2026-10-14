//go:build linux

package builtin

import (
	"bytes"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// sysPlatform 取 sys.info 里 Linux 特有的字段。
func sysPlatform() sysPlatformInfo {
	var p sysPlatformInfo
	var u unix.Utsname
	if err := unix.Uname(&u); err == nil {
		p.OSVersion = cString(u.Release[:])
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

// cString 把以 0 结尾的字节数组转成字符串。
func cString(s []byte) string {
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}
