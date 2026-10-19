package cli

import (
	"context"
	"errors"
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

type keeperFixture struct {
	k         *nodeKeeper
	st        *coreState
	reachable bool
	launches  int
	launchErr error
	runs      []*embeddedRun
	clock     time.Time
}

func newKeeperFixture() *keeperFixture {
	f := &keeperFixture{st: &coreState{}, clock: time.Unix(1000, 0)}
	f.k = newNodeKeeper("unused.sock", f.st)
	f.k.reachable = func() bool { return f.reachable }
	f.k.now = func() time.Time { return f.clock }
	f.k.launch = func(context.Context) (*embeddedRun, error) {
		f.launches++
		if f.launchErr != nil {
			return &embeddedRun{logPath: "ui-node.log"}, f.launchErr
		}
		r := &embeddedRun{done: make(chan struct{}), result: make(chan error, 1), logPath: "ui-node.log"}
		f.runs = append(f.runs, r)
		return r, nil
	}
	return f
}

func TestKeeperBackoff(t *testing.T) {
	f := newKeeperFixture()
	f.launchErr = errors.New("boom")
	ctx := context.Background()

	if err := f.k.step(ctx); err == nil {
		t.Fatal("启动失败应返回错误")
	}
	if s := f.st.snapshot(); s.Mode != "none" || s.NodeError != "boom" || s.Log != "ui-node.log" {
		t.Fatalf("状态不对: %+v", s)
	}
	// 退避期内不再启动。
	if err := f.k.step(ctx); err != nil || f.launches != 1 {
		t.Fatalf("退避期内不应启动: err=%v launches=%d", err, f.launches)
	}
	// 退避间隔依次为 2、4、8、16、30、30 秒。
	for i, want := range []time.Duration{2, 4, 8, 16, 30, 30} {
		d := want * time.Second
		f.clock = f.clock.Add(d - time.Millisecond)
		before := f.launches
		f.k.step(ctx)
		if f.launches != before {
			t.Fatalf("第 %d 轮：退避未到不应启动", i)
		}
		f.clock = f.clock.Add(time.Millisecond)
		f.k.step(ctx)
		if f.launches != before+1 {
			t.Fatalf("第 %d 轮：退避到期应启动", i)
		}
	}
}

func TestKeeperRestartAndExternal(t *testing.T) {
	f := newKeeperFixture()
	ctx := context.Background()

	if err := f.k.step(ctx); err != nil {
		t.Fatal(err)
	}
	if s := f.st.snapshot(); s.Mode != "embedded" || s.NodeError != "" {
		t.Fatalf("应为 embedded: %+v", s)
	}
	f.k.step(ctx)
	if f.launches != 1 {
		t.Fatalf("运行中不应重复启动: %d", f.launches)
	}

	// 内置节点中途退出：立即重新拉起。
	f.runs[0].result <- errors.New("崩了")
	close(f.runs[0].done)
	if err := f.k.step(ctx); err != nil {
		t.Fatal(err)
	}
	if f.launches != 2 || f.st.snapshot().Mode != "embedded" {
		t.Fatalf("应重新拉起: launches=%d %+v", f.launches, f.st.snapshot())
	}

	// 别的节点占了本机接口：改为复用，不再启动。
	f.runs[1].result <- nil
	close(f.runs[1].done)
	f.reachable = true
	if err := f.k.step(ctx); err != nil {
		t.Fatal(err)
	}
	if s := f.st.snapshot(); s.Mode != "external" || s.NodeError != "" || f.launches != 2 {
		t.Fatalf("应为 external: %+v launches=%d", s, f.launches)
	}

	// 外部节点消失：拉起内置节点。
	f.reachable = false
	if err := f.k.step(ctx); err != nil {
		t.Fatal(err)
	}
	if f.launches != 3 || f.st.snapshot().Mode != "embedded" {
		t.Fatalf("外部节点消失后应拉起内置节点: launches=%d", f.launches)
	}
}
