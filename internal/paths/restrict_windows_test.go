//go:build windows

package paths

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// aceHeader 是 ACE 的公共头。
type aceHeader struct {
	Type  byte
	Flags byte
	Size  uint16
}

// TestRestrictFileSocket 创建 socket 后读回 DACL：受保护（不继承），
// 且只有当前用户和 SYSTEM 两个允许项。
func TestRestrictFileSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	if err := RestrictFile(path); err != nil {
		t.Fatalf("RestrictFile: %v", err)
	}
	checkDACL(t, path, true, false)

	// 当前用户仍能连接。
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("设置 DACL 后当前用户连接失败: %v", err)
	}
	c.Close()
}

// TestRestrictDirInherited 收紧已有目录后，目录受保护；目录里已有的和新建
// 的文件只继承当前用户和 SYSTEM 两项。
func TestRestrictDirInherited(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(old, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestrictDir(dir); err != nil {
		t.Fatalf("RestrictDir: %v", err)
	}
	checkDACL(t, dir, true, false)
	checkDACL(t, old, false, true)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(sub, "new.txt")
	if err := os.WriteFile(newFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkDACL(t, newFile, false, true)
}

// TestEnsureHomeRestricts 验证 EnsureHome 创建并收紧数据目录。
func TestEnsureHomeRestricts(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DESKBRIDGE_HOME", home)
	got, err := EnsureHome()
	if err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	if got != home {
		t.Fatalf("EnsureHome() = %q，期望 %q", got, home)
	}
	checkDACL(t, home, true, false)
}

// checkDACL 读回 path 的 DACL：只能有当前用户和 SYSTEM 的允许项，且两者都有；
// protected 为 true 时要求 DACL 受保护，inherited 为 true 时要求每一项都是
// 继承来的，否则要求都是显式的。
func checkDACL(t *testing.T, path string, protected, inherited bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("读取安全描述符失败: %v", err)
	}
	ctrl, _, err := sd.Control()
	if err != nil {
		t.Fatalf("读取控制位失败: %v", err)
	}
	if protected && ctrl&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("DACL 未设置为受保护（控制位 %#x）", ctrl)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("读取 DACL 失败: %v（%v）", dacl, err)
	}

	user, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatalf("GetAce(%d): %v", i, err)
		}
		hdr := (*aceHeader)(unsafe.Pointer(ace))
		if hdr.Type != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Errorf("第 %d 项不是允许项：类型 %d", i, hdr.Type)
		}
		if got := hdr.Flags&windows.INHERITED_ACE != 0; got != inherited {
			t.Errorf("%s 第 %d 项继承标志为 %v，期望 %v", path, i, got, inherited)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user) && !sid.Equals(system) {
			t.Errorf("出现了预期之外的 SID：%s", sid)
		}
		seen[sid.String()] = true
	}
	if !seen[user.String()] || !seen[system.String()] {
		t.Errorf("%s 的 DACL 缺少当前用户或 SYSTEM：%v", path, seen)
	}
}

func TestRestrictFileMissing(t *testing.T) {
	if err := RestrictFile(filepath.Join(os.TempDir(), "db-no-such-socket-file")); err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
}
