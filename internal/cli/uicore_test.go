package cli

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNodeReachable(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "n.sock")
	if nodeReachable(sock, 200*time.Millisecond) {
		t.Fatal("socket 不存在时不应可达")
	}

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	if !nodeReachable(sock, time.Second) {
		t.Fatal("有监听者时应可达")
	}
	ln.Close()

	// 普通文件，没人监听
	plain := filepath.Join(dir, "plain.sock")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if nodeReachable(plain, 200*time.Millisecond) {
		t.Fatal("没人监听的文件不应可达")
	}
	// 监听者关闭后 socket 文件可能残留
	if nodeReachable(sock, 200*time.Millisecond) {
		t.Fatal("监听者关闭后不应可达")
	}
}

func TestExited(t *testing.T) {
	if !exited(nil) {
		t.Fatal("没有启动的节点应视为已退出")
	}
	done := make(chan struct{})
	if exited(done) {
		t.Fatal("运行中的节点不应视为已退出")
	}
	close(done)
	if !exited(done) {
		t.Fatal("done 关闭后应视为已退出")
	}
}
