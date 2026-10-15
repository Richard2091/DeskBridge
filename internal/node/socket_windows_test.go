//go:build windows

package node

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

// TestChmodSocketRestrictsDACL 创建 socket 后读回 DACL：受保护（不继承），
// 且只有当前用户和 SYSTEM 两个允许项。
func TestChmodSocketRestrictsDACL(t *testing.T) {
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
	if err := chmodSocket(path); err != nil {
		t.Fatalf("chmodSocket: %v", err)
	}

	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("读取安全描述符失败: %v", err)
	}
	ctrl, _, err := sd.Control()
	if err != nil {
		t.Fatalf("读取控制位失败: %v", err)
	}
	if ctrl&windows.SE_DACL_PROTECTED == 0 {
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
		if hdr.Flags&windows.INHERITED_ACE != 0 {
			t.Errorf("第 %d 项是继承来的", i)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user) && !sid.Equals(system) {
			t.Errorf("出现了预期之外的 SID：%s", sid)
		}
		seen[sid.String()] = true
	}
	if !seen[user.String()] {
		t.Error("DACL 里没有当前用户")
	}

	// 当前用户仍能连接。
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("设置 DACL 后当前用户连接失败: %v", err)
	}
	c.Close()
}

func TestChmodSocketMissingFile(t *testing.T) {
	if err := chmodSocket(filepath.Join(os.TempDir(), "db-no-such-socket-file")); err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
}
