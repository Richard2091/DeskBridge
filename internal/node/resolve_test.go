package node

// 目标解析、共享传输和探测超时的测试。

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/mesh/meshtest"
)

// startNodeWith 同 startNodeChannels，但接受任意 mesh.Channel。
func startNodeWith(t *testing.T, name string, chs ...mesh.Channel) string {
	t.Helper()
	socket := filepath.Join(shortTempDir(t), name+".sock")
	n := New(Config{Channels: chs, Device: testDevice(t), Name: name, Socket: socket})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if err := n.Run(ctx); err != nil {
			t.Errorf("节点退出: %v", err)
		}
	}()
	waitFor(t, func() bool {
		_, err := os.Stat(socket)
		return err == nil
	}, "节点没有在超时内监听 socket")
	return socket
}

func connectedFake() *meshtest.Fake {
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{
		Kind: mesh.KindTailscale, State: mesh.StateConnected,
		Self: &mesh.Peer{ID: "self-1", Name: "desk-pc", HostName: "DESK-PC", Addrs: []string{"100.64.0.1"}, Mine: true, Online: true},
	})
	return fake
}

func capName(t *testing.T, c *client.Client, target string) string {
	t.Helper()
	capability, err := c.Capability(context.Background(), target)
	if err != nil {
		t.Fatalf("目标 %q：%v", target, err)
	}
	return capability.Name
}

func TestResolveHostNameDuplicatesPreferOnline(t *testing.T) {
	fake := connectedFake()
	offAddr, _ := capServer(t, "dev-off", "off-box", 0)
	onAddr, _ := capServer(t, "dev-on", "on-box", 0)
	otherAddr, _ := capServer(t, "dev-other", "other-box", 0)
	fake.SetRoute("100.64.0.11:7465", offAddr)
	fake.SetRoute("100.64.0.12:7465", onAddr)
	fake.SetRoute("100.64.0.13:7465", otherAddr)
	fake.SetPeers([]mesh.Peer{
		{ID: "p1", Name: "a-off", HostName: "DUP", Addrs: []string{"100.64.0.11"}, Mine: true, Online: false},
		{ID: "p2", Name: "b-on", HostName: "dup", Addrs: []string{"100.64.0.12"}, Mine: true, Online: true},
		// 主机名为 b-on 的另一台：精确的 Name 匹配优先于主机名匹配。
		{ID: "p3", Name: "c-other", HostName: "b-on", Addrs: []string{"100.64.0.13"}, Mine: true, Online: true},
	})
	c := client.New(startNodeWith(t, "win-pc", fake))

	if got := capName(t, c, "dup"); got != "on-box" {
		t.Errorf("主机名重复时应选在线节点，得到 %q", got)
	}
	if got := capName(t, c, "b-on"); got != "on-box" {
		t.Errorf("精确 Name 应优先于别的节点的主机名，得到 %q", got)
	}
	if got := capName(t, c, "a-off"); got != "off-box" {
		t.Errorf("唯一匹配的离线节点仍应能解析，得到 %q", got)
	}
}

func TestResolveByDisplayName(t *testing.T) {
	fake := connectedFake()
	bobAddr, _ := capServer(t, "dev-bob", "Bob 工作站", 0)
	namedAddr, _ := capServer(t, "dev-named", "named-box", 0)
	fake.SetRoute("100.64.0.11:7465", bobAddr)
	fake.SetRoute("100.64.0.12:7465", namedAddr)
	fake.SetPeers([]mesh.Peer{
		{ID: "p1", Name: "b-run", HostName: "BOB-PC", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true},
		// 显示名称优先于别的节点的主机名。
		{ID: "p2", Name: "c-host", HostName: "bob 工作站", Addrs: []string{"100.64.0.12"}, Mine: true, Online: true},
	})
	c := client.New(startNodeWith(t, "我的电脑", fake))

	// 缓存里还没有显示名称，但已有 Name/主机名匹配时不探测。
	if got := capName(t, c, "bob 工作站"); got != "named-box" {
		t.Errorf("没有缓存时按主机名匹配，得到 %q", got)
	}
	// 本机的显示名称（含中文）。
	if got := capName(t, c, "我的电脑"); got != "我的电脑" {
		t.Errorf("本机显示名称应解析为本机，得到 %q", got)
	}
	res, err := c.Probe(context.Background(), "我的电脑")
	if err != nil || !res.Self {
		t.Errorf("Probe(本机显示名称) = %+v，%v", res, err)
	}
	// 列设备会探测并缓存显示名称；之后显示名称优先于主机名。
	if _, err := c.Devices(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := capName(t, c, "bob 工作站"); got != "Bob 工作站" {
		t.Errorf("缓存后显示名称应优先于主机名，得到 %q", got)
	}
}

func TestResolveDisplayNameProbesOnCacheMiss(t *testing.T) {
	fake := connectedFake()
	bobAddr, hits := capServer(t, "dev-bob", "Bob 工作站", 0)
	fake.SetRoute("100.64.0.11:7465", bobAddr)
	fake.SetPeers([]mesh.Peer{
		{ID: "p1", Name: "b-run", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true},
		{ID: "p2", Name: "z-off", Addrs: []string{"100.64.0.12"}, Mine: true, Online: false},
	})
	c := client.New(startNodeWith(t, "win-pc", fake))

	if got := capName(t, c, "Bob 工作站"); got != "Bob 工作站" {
		t.Errorf("缓存未命中时应探测并按显示名称匹配，得到 %q", got)
	}
	// 解析时探测一次，转发一次；之后探测走缓存。
	if _, err := c.Capability(context.Background(), "Bob 工作站"); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("对端收到 %d 个请求，期望 3（探测 1 + 转发 2）", got)
	}
}

func TestResolveNotFound(t *testing.T) {
	fake := connectedFake()
	fake.SetPeers([]mesh.Peer{{ID: "p1", Name: "no-addr", Mine: true, Online: true}})
	c := client.New(startNodeWith(t, "win-pc", fake))
	if _, err := c.Capability(context.Background(), "no-addr"); err == nil {
		t.Error("没有地址的对端不应作为候选")
	}
	if _, err := c.Capability(context.Background(), "nobody"); err == nil {
		t.Error("不存在的设备应报错")
	}
}

// countingChannel 统计 Dial 次数。
type countingChannel struct {
	*meshtest.Fake
	dials atomic.Int32
}

func (c *countingChannel) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	c.dials.Add(1)
	return c.Fake.Dial(ctx, network, addr)
}

func TestForwardReusesSharedTransport(t *testing.T) {
	fake := connectedFake()
	addr, hits := capServer(t, "dev-run", "Bob", 0)
	fake.SetRoute("100.64.0.11:7465", addr)
	fake.SetPeers([]mesh.Peer{{ID: "p1", Name: "b-run", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true}})
	ch := &countingChannel{Fake: fake}
	c := client.New(startNodeWith(t, "win-pc", ch))

	for i := 0; i < 5; i++ {
		if got := capName(t, c, "b-run"); got != "Bob" {
			t.Fatalf("第 %d 次转发得到 %q", i, got)
		}
	}
	if _, err := c.Devices(context.Background(), true); err != nil { // 探测也走同一个传输
		t.Fatal(err)
	}
	if got := hits.Load(); got < 6 {
		t.Errorf("对端只收到 %d 个请求", got)
	}
	if got := ch.dials.Load(); got != 1 {
		t.Errorf("转发和探测应复用同一条连接，实际拨号 %d 次", got)
	}
}

// statusCountingChannel 统计 Status/Peers/Snapshot 调用次数。
type statusCountingChannel struct {
	*meshtest.Fake
	statuses, peers, snapshots atomic.Int32
}

func (c *statusCountingChannel) Status(ctx context.Context) (mesh.Status, error) {
	c.statuses.Add(1)
	return c.Fake.Status(ctx)
}

func (c *statusCountingChannel) Peers(ctx context.Context) ([]mesh.Peer, error) {
	c.peers.Add(1)
	return c.Fake.Peers(ctx)
}

func (c *statusCountingChannel) Snapshot(ctx context.Context) (mesh.Status, []mesh.Peer, error) {
	c.snapshots.Add(1)
	return c.Fake.Snapshot(ctx)
}

func TestResolveQueriesChannelOnce(t *testing.T) {
	fake := connectedFake()
	addr, _ := capServer(t, "dev-run", "Bob", 0)
	fake.SetRoute("100.64.0.11:7465", addr)
	fake.SetPeers([]mesh.Peer{{ID: "p1", Name: "b-run", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true}})
	ch := &statusCountingChannel{Fake: fake}
	n := New(Config{Channels: []mesh.Channel{ch}, Device: testDevice(t), Name: "win-pc"})

	for _, target := range []string{"b-run", "desk-pc"} {
		before := ch.snapshots.Load()
		if _, ok := n.resolveTarget(context.Background(), target); !ok {
			t.Fatalf("目标 %q 没有解析到", target)
		}
		if got := ch.snapshots.Load() - before; got != 1 {
			t.Errorf("目标 %q：查询 %d 次，期望 1", target, got)
		}
	}
	if ch.statuses.Load() != 0 || ch.peers.Load() != 0 {
		t.Errorf("解析不应单独调用 Status/Peers: %d/%d", ch.statuses.Load(), ch.peers.Load())
	}
}

// slowProbeChannel 的 Probe 一直阻塞到 context 结束。
type slowProbeChannel struct{ *meshtest.Fake }

func (c slowProbeChannel) Probe(ctx context.Context, _ string) (mesh.Peer, error) {
	<-ctx.Done()
	return mesh.Peer{}, ctx.Err()
}

func TestProbeTimeoutStillReturnsPartialResult(t *testing.T) {
	old := pathProbeTimeout
	pathProbeTimeout = 100 * time.Millisecond
	t.Cleanup(func() { pathProbeTimeout = old })

	fake := connectedFake()
	addr, _ := capServer(t, "dev-run", "Bob", 0)
	fake.SetRoute("100.64.0.11:7465", addr)
	fake.SetPeers([]mesh.Peer{{ID: "p1", Name: "b-run", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true}})
	c := client.New(startNodeWith(t, "win-pc", slowProbeChannel{fake}))

	start := time.Now()
	res, err := c.Probe(context.Background(), "b-run")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("路径测量超时后没有及时返回: %v", elapsed)
	}
	if !res.DeskBridge || res.Peer.Name != "b-run" {
		t.Errorf("超时后应仍有能力探测结果: %+v", res)
	}
	if !strings.Contains(res.Error, "测量路径失败") {
		t.Errorf("Error = %q，应说明路径测量失败", res.Error)
	}
}

func TestProbeClassifiesDialErrors(t *testing.T) {
	fake := connectedFake()
	fake.SetRoute("100.64.0.12:7465", closedAddr(t))
	fake.SetPeers([]mesh.Peer{
		{ID: "p-down", Name: "c-down", Addrs: []string{"100.64.0.12"}, Mine: true, Online: true},
		{ID: "p-nr", Name: "d-noroute", Addrs: []string{"100.64.0.13"}, Mine: true, Online: true},
	})
	c := client.New(startNodeWith(t, "win-pc", fake))

	res, err := c.Probe(context.Background(), "c-down")
	if err != nil || res.DeskBridge || !strings.Contains(res.Error, "未运行 DeskBridge") {
		t.Errorf("连接被拒绝应分类为未运行 DeskBridge: %+v，%v", res, err)
	}
	res, err = c.Probe(context.Background(), "d-noroute")
	if err != nil || res.DeskBridge || !strings.Contains(res.Error, "连接方式未连接") {
		t.Errorf("ErrNotConnected 应分类为连接方式未连接: %+v，%v", res, err)
	}
	list, err := c.Devices(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if d := findDevice(t, list.Groups[0], "d-noroute"); d.ProbeError != "连接方式未连接" {
		t.Errorf("设备列表里 d-noroute 的 ProbeError = %q", d.ProbeError)
	}
	if d := findDevice(t, list.Groups[0], "c-down"); d.ProbeError != "未运行 DeskBridge" {
		t.Errorf("设备列表里 c-down 的 ProbeError = %q", d.ProbeError)
	}
}
