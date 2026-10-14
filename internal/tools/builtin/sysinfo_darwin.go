//go:build darwin

package builtin

import (
	"os"
	"os/user"
	"syscall"
)

// sysPlatform 取 sys.info 里 macOS 特有的字段。取不到的字段按规范省略。
func sysPlatform() sysPlatformInfo {
	var p sysPlatformInfo
	if u, err := syscall.Uname(); err == nil {
		p.OSVersion = int8StringDarwin(u.Release[:])
	}
	if cu, err := user.Current(); err == nil {
		p.User = cu.Username
		p.Home = cu.HomeDir
	}
	if n, err := syscall.SysctlUint64("hw.memsize"); err == nil {
		p.MemTotal = int64(n)
	}
	return p
}

// int8StringDarwin 把 Uname 里的 int8 数组转成到第一个 0 为止的字符串。
func int8StringDarwin(s []int8) string {
	b := make([]byte, 0, len(s))
	for _, c := range s {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}
