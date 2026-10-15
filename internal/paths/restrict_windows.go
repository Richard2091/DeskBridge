//go:build windows

package paths

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Windows 的访问控制靠 DACL，Go 的 0600/0700 权限位不起作用。默认 DACL
// 继承自父目录，常常允许同机其他用户读写；本机接口 socket 也是文件系统
// 对象（重解析点），连接它需要对文件有访问权限。所以这里给数据目录和
// socket 设置受保护的 DACL（不再继承父目录），只授予当前进程用户和
// SYSTEM 完全控制。

// RestrictDir 限制目录只有当前用户和 SYSTEM 能访问，目录里已有和以后
// 新建的文件、子目录都继承同样的权限。
func RestrictDir(dir string) error {
	return restrict(dir, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
}

// RestrictFile 限制文件只有当前用户和 SYSTEM 能访问。
func RestrictFile(path string) error {
	return restrict(path, windows.NO_INHERITANCE)
}

func restrict(path string, inherit uint32) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("构造 SYSTEM SID 失败: %w", err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		fullControlFor(user, windows.TRUSTEE_IS_USER, inherit),
		fullControlFor(system, windows.TRUSTEE_IS_WELL_KNOWN_GROUP, inherit),
	}, nil)
	if err != nil {
		return fmt.Errorf("构造 %s 的访问控制列表失败: %w", path, err)
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
	if err != nil {
		return fmt.Errorf("设置 %s 的访问权限失败: %w", path, err)
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

func fullControlFor(sid *windows.SID, typ windows.TRUSTEE_TYPE, inherit uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inherit,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  typ,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
