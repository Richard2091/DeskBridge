//go:build !windows

package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureHomeRestricts 验证 EnsureHome 把已存在的宽松目录也收紧到 0700。
func TestEnsureHomeRestricts(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DESKBRIDGE_HOME", home)
	if _, err := EnsureHome(); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	fi, err := os.Stat(home)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("目录权限 %o，期望 700", perm)
	}
}
