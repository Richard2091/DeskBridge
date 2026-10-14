//go:build darwin

package builtin

import (
	"bytes"
	"os/user"

	"golang.org/x/sys/unix"
)

// sysPlatform 取 sys.info 里 macOS 特有的字段。取不到的字段按规范省略。
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
	if n, err := unix.SysctlUint64("hw.memsize"); err == nil {
		p.MemTotal = int64(n)
	}
	return p
}

// cString 把以 0 结尾的字节数组转成字符串。
func cString(s []byte) string {
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}
