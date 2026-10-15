//go:build windows

package node

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// chmodSocket 限制 socket 文件只有当前用户（和 SYSTEM）能访问。
//
// Windows 的 AF_UNIX socket 是文件系统里的对象（重解析点），访问控制
// 靠文件的 DACL，默认继承父目录的 ACL，常常允许同机其他用户读写。能连
// 上本机接口就获得 Mine:true 的本机身份，所以这里给文件设置受保护的
// DACL（PROTECTED_DACL_SECURITY_INFORMATION，不再继承父目录），只授予当前
// 进程用户和 SYSTEM 完全控制。连接 AF_UNIX socket 需要对该文件有访问权限，
// 没有被授权的用户连接时会被系统拒绝。设置失败必须返回错误，不能静默放行。
func chmodSocket(path string) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("构造 SYSTEM SID 失败: %w", err)
	}
	entries := []windows.EXPLICIT_ACCESS{
		fullControlFor(user, windows.TRUSTEE_IS_USER),
		fullControlFor(system, windows.TRUSTEE_IS_WELL_KNOWN_GROUP),
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("构造 socket 文件的访问控制列表失败: %w", err)
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
	if err != nil {
		return fmt.Errorf("设置 socket 文件 %s 的访问权限失败: %w", path, err)
	}
	return nil
}

// currentUserSID 返回当前进程令牌的用户 SID。
func currentUserSID() (*windows.SID, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("取当前用户 SID 失败: %w", err)
	}
	return tu.User.Sid, nil
}

func fullControlFor(sid *windows.SID, typ windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  typ,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
