//go:build windows

package builtin

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sysPlatform 取 sys.info 里 Windows 特有的字段。
func sysPlatform() sysPlatformInfo {
	var p sysPlatformInfo
	p.OSVersion = windowsOSVersion()
	p.User = windowsCurrentUser()
	if home, err := os.UserHomeDir(); err == nil {
		p.Home = home
	}

	var ms windows.MemoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if err := windows.GlobalMemoryStatusEx(&ms); err == nil {
		p.MemTotal = int64(ms.TotalPhys)
		p.MemAvail = int64(ms.AvailPhys)
	}

	// GetTickCount64 是开机以来的毫秒数。
	p.BootTime = time.Now().Add(-time.Duration(windows.GetTickCount64()) * time.Millisecond)

	p.Codepages = &codePages{ANSI: int(windows.GetACP()), OEM: int(windows.GetOEMCP())}

	var sid uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sid); err == nil {
		p.Session = &sessionInfo{
			ID:          int(sid),
			Interactive: sid == windows.WTSGetActiveConsoleSessionId(),
		}
	}
	return p
}

// windowsOSVersion 返回系统版本，如 10.0.19045。
func windowsOSVersion() string {
	var v windows.OsVersionInfo
	v.OSVersionInfoSize = uint32(unsafe.Sizeof(v))
	if err := windows.RtlGetVersion(&v); err != nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}

// windowsCurrentUser 返回节点进程的用户，格式为 域\用户。
func windowsCurrentUser() string {
	token := windows.GetCurrentProcessToken()
	defer token.Close()
	tu, err := token.GetTokenUser()
	if err != nil {
		return ""
	}
	domain, account, _, err := tu.User.Sid.LookupAccount("")
	if err != nil {
		return ""
	}
	if domain != "" {
		return domain + "\\" + account
	}
	return account
}
