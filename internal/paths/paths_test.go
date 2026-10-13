package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHomeWithEnv(t *testing.T) {
	t.Setenv("DESKBRIDGE_HOME", filepath.Join("some", "dir"))
	home, err := Home()
	if err != nil {
		t.Fatalf("Home() 出错: %v", err)
	}
	if home != filepath.Join("some", "dir") {
		t.Errorf("Home() = %q, 期望 some%cdir", home, os.PathSeparator)
	}
}

func TestHomeWithoutEnv(t *testing.T) {
	t.Setenv("DESKBRIDGE_HOME", "")
	home, err := Home()
	if err != nil {
		t.Fatalf("Home() 出错: %v", err)
	}
	// 只检查最后一段目录名：Windows、macOS 是 DeskBridge，Linux 是 deskbridge。
	want := "deskbridge"
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		want = "DeskBridge"
	}
	if filepath.Base(home) != want {
		t.Errorf("Home() = %q, 末段期望 %q", home, want)
	}
}

func TestSubPaths(t *testing.T) {
	t.Setenv("DESKBRIDGE_HOME", filepath.Join("some", "dir"))
	sep := string(os.PathSeparator)
	for _, c := range []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"TailscaleDir", TailscaleDir, filepath.Join("some", "dir") + sep + "tailscale"},
		{"SocketPath", SocketPath, filepath.Join("some", "dir") + sep + "node.sock"},
		{"DeviceKeyFile", DeviceKeyFile, filepath.Join("some", "dir") + sep + "device.key"},
	} {
		got, err := c.got()
		if err != nil {
			t.Fatalf("%s() 出错: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s() = %q, 期望 %q", c.name, got, c.want)
		}
	}
}

func TestEnsure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := Ensure(dir); err != nil {
		t.Fatalf("Ensure() 出错: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("目录未创建: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("目录权限 = %o, 期望 700", perm)
		}
	}
	if err := Ensure(dir); err != nil {
		t.Errorf("对已存在目录 Ensure() 不应报错: %v", err)
	}
}
