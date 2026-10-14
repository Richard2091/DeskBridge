package node

// 设备列表（GET /local/devices）的测试：用 meshtest.Fake 构造对端，对端的
// DeskBridge 用 httptest 服务假装，经 Fake 的路由拨号。

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/mesh/meshtest"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// capServer 起一个假的对端节点，返回能力报告；delay 为响应前的等待
// （等待中请求被取消就立即返回）。hits 统计收到的请求数。
func capServer(t *testing.T, deviceID, name string, delay time.Duration) (addr string, hits *atomic.Int32) {
	t.Helper()
	hits = new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		writeJSON(w, http.StatusOK, protocol.Capability{
			Protocol: protocol.Version,
			DeviceID: deviceID,
			Name:     name,
			Version:  "0.0.1",
			Platform: protocol.Platform{OS: "windows", Arch: "amd64"},
			Tools:    []protocol.ToolSpec{{Name: "sys.info"}, {Name: "fs.read"}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String(), hits
}

// closedAddr 返回一个没有人监听的本机地址（连接会被拒绝）。
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// startNodeChannels 用现成的若干 Fake 启动节点（Fake 的状态由调用方预设）。
func startNodeChannels(t *testing.T, name string, fakes ...*meshtest.Fake) string {
	t.Helper()
	chs := make([]mesh.Channel, len(fakes))
	for i, f := range fakes {
		chs[i] = f
	}
	socket := filepath.Join(shortTempDir(t), name+".sock")
	n := New(Config{Channels: chs, Device: testDevice(t), Name: name, Socket: socket})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = n.Run(ctx) }()
	waitFor(t, func() bool {
		_, err := os.Stat(socket)
		return err == nil
	}, "节点没有在超时内监听 socket")
	return socket
}

// devicesFixture 是设备列表测试的公共场景。
type devicesFixture struct {
	socket  string
	runHits *atomic.Int32 // 运行着 DeskBridge 的对端 b-run 收到的探测数
	dupHits [2]*atomic.Int32
}

// newDevicesFixture 构造：本机；在线运行 DeskBridge（b-run）；在线没运行（c-down）；
// 离线（a-off）；非同账号（x-other）；同一设备 ID 的两个节点（d-dup-direct 直连、
// e-dup-relay 中继）。
func newDevicesFixture(t *testing.T) *devicesFixture {
	t.Helper()
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{
		Kind: mesh.KindTailscale, State: mesh.StateConnected, Account: "alice@example.com",
		Self: &mesh.Peer{ID: "self-1", Name: "win-pc", HostName: "WIN-PC", Addrs: []string{"100.64.0.1"},
			OS: "windows", Mine: true, Online: true, Client: mesh.ClientDeskBridge},
	})
	f := &devicesFixture{}
	runAddr, runHits := capServer(t, "dev-run", "Bob 工作站", 0)
	f.runHits = runHits
	dup1Addr, dup1Hits := capServer(t, "dev-dup", "Dup", 0)
	dup2Addr, dup2Hits := capServer(t, "dev-dup", "Dup", 0)
	f.dupHits = [2]*atomic.Int32{dup1Hits, dup2Hits}
	fake.SetRoute("100.64.0.11:7465", runAddr)
	fake.SetRoute("100.64.0.12:7465", closedAddr(t))
	fake.SetRoute("100.64.0.14:7465", dup1Addr)
	fake.SetRoute("100.64.0.15:7465", dup2Addr)
	fake.SetPeers([]mesh.Peer{
		{ID: "p-off", Name: "a-off", Addrs: []string{"100.64.0.13"}, Mine: true, Online: false, Path: mesh.PathUnknown},
		{ID: "p-dup2", Name: "e-dup-relay", Addrs: []string{"100.64.0.15"}, Mine: true, Online: true, Path: mesh.PathRelay, Relay: "hkg"},
		{ID: "p-other", Name: "x-other", Addrs: []string{"100.64.0.16"}, Mine: false, Online: true},
		{ID: "p-down", Name: "c-down", Addrs: []string{"100.64.0.12"}, Mine: true, Online: true, Client: mesh.ClientTailscale},
		{ID: "p-run", Name: "b-run", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true, Path: mesh.PathLAN, Client: mesh.ClientDeskBridge},
		{ID: "p-dup1", Name: "d-dup-direct", Addrs: []string{"100.64.0.14"}, Mine: true, Online: true, Path: mesh.PathDirect},
	})
	f.socket = startNodeChannels(t, "win-pc", fake)
	return f
}

func deviceNames(g client.DeviceGroup) []string {
	names := make([]string, len(g.Devices))
	for i, d := range g.Devices {
		names[i] = d.Name
	}
	return names
}

func findDevice(t *testing.T, g client.DeviceGroup, name string) client.Device {
	t.Helper()
	for _, d := range g.Devices {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("设备列表里没有 %s: %v", name, deviceNames(g))
	return client.Device{}
}

func TestDevicesGroupSortAndProbe(t *testing.T) {
	f := newDevicesFixture(t)
	c := client.New(f.socket)
	list, err := c.Devices(context.Background(), false)
	if err != nil {
		t.Fatalf("Devices() 出错: %v", err)
	}
	if len(list.Groups) != 1 || list.Groups[0].Kind != mesh.KindTailscale {
		t.Fatalf("分组不对: %+v", list.Groups)
	}
	g := list.Groups[0]
	if g.Status.Self != nil || g.Status.State != mesh.StateConnected || g.Status.Account != "alice@example.com" {
		t.Errorf("组状态不对（应保留状态、去掉 self）: %+v", g.Status)
	}

	// 本机第一，在线的在前并按显示名称排序（Bob 工作站、c-down、Dup），离线最后；
	// 非同账号的 x-other 不出现；同一设备 ID 的两个节点只剩一条。
	want := []string{"win-pc", "b-run", "c-down", "d-dup-direct", "a-off"}
	if got := deviceNames(g); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] || got[4] != want[4] {
		t.Fatalf("设备顺序 = %v, 期望 %v", got, want)
	}

	self := g.Devices[0]
	if !self.Self || !self.Online || self.DeskBridge == nil || self.DeskBridge.DisplayName != "win-pc" || self.Client != mesh.ClientDeskBridge {
		t.Errorf("本机条目不对: %+v", self)
	}
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if self.DeskBridge.DeviceID != st.DeviceID {
		t.Errorf("本机 device_id = %q, 期望 %q", self.DeskBridge.DeviceID, st.DeviceID)
	}

	run := findDevice(t, g, "b-run")
	if run.DeskBridge == nil || run.DeskBridge.DeviceID != "dev-run" || run.DeskBridge.DisplayName != "Bob 工作站" ||
		run.DeskBridge.Tools != 2 || run.DeskBridge.Platform.OS != "windows" || run.ProbeError != "" {
		t.Errorf("b-run = %+v", run)
	}
	if run.Path != mesh.PathLAN || run.Client != mesh.ClientDeskBridge {
		t.Errorf("连接层信息没带上: %+v", run)
	}

	down := findDevice(t, g, "c-down")
	if down.DeskBridge != nil || down.ProbeError != "未运行 DeskBridge" || down.Client != mesh.ClientTailscale {
		t.Errorf("c-down = %+v", down)
	}

	off := findDevice(t, g, "a-off")
	if off.DeskBridge != nil || off.ProbeError != "" || off.Online {
		t.Errorf("离线设备不应探测: %+v", off)
	}

	dup := findDevice(t, g, "d-dup-direct")
	if dup.DeskBridge == nil || len(dup.Duplicates) != 1 || dup.Duplicates[0] != "e-dup-relay" {
		t.Errorf("去重结果不对: %+v", dup)
	}

	// JSON 里没有探测结果的字段应省略，而不是空对象。
	raw, _ := json.Marshal(down)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["deskbridge"]; has {
		t.Errorf("没运行 DeskBridge 时不应有 deskbridge 字段: %s", raw)
	}
}

func TestDevicesCacheAndRefresh(t *testing.T) {
	f := newDevicesFixture(t)
	c := client.New(f.socket)
	for i := 0; i < 2; i++ {
		if _, err := c.Devices(context.Background(), false); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.runHits.Load(); got != 1 {
		t.Errorf("30 秒内重复列出，探测次数 = %d, 期望 1（走缓存）", got)
	}
	if _, err := c.Devices(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := f.runHits.Load(); got != 2 {
		t.Errorf("refresh=1 后探测次数 = %d, 期望 2", got)
	}
}

func TestDevicesConcurrentRequestsShareOneProbe(t *testing.T) {
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	addr, hits := capServer(t, "dev-run", "Bob", 300*time.Millisecond)
	fake.SetRoute("100.64.0.11:7465", addr)
	fake.SetPeers([]mesh.Peer{{ID: "p-run", Name: "b-run", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true}})
	c := client.New(startNodeChannels(t, "win-pc", fake))

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list, err := c.Devices(context.Background(), false)
			if err != nil {
				t.Errorf("Devices() 出错: %v", err)
				return
			}
			if d := findDevice(t, list.Groups[0], "b-run"); d.DeskBridge == nil {
				t.Errorf("并发请求没拿到探测结果: %+v", d)
			}
		}()
	}
	wg.Wait()
	if got := hits.Load(); got != 1 {
		t.Errorf("并发 6 个请求，探测次数 = %d, 期望 1", got)
	}
}

func TestDevicesProbeTimeoutDoesNotBlock(t *testing.T) {
	old := probeTimeout
	probeTimeout = 150 * time.Millisecond
	t.Cleanup(func() { probeTimeout = old })

	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	slowAddr, _ := capServer(t, "dev-slow", "Slow", 5*time.Second)
	fastAddr, _ := capServer(t, "dev-fast", "Fast", 0)
	fake.SetRoute("100.64.0.11:7465", slowAddr)
	fake.SetRoute("100.64.0.12:7465", fastAddr)
	fake.SetPeers([]mesh.Peer{
		{ID: "p-slow", Name: "slow", Addrs: []string{"100.64.0.11"}, Mine: true, Online: true},
		{ID: "p-fast", Name: "fast", Addrs: []string{"100.64.0.12"}, Mine: true, Online: true},
	})
	c := client.New(startNodeChannels(t, "win-pc", fake))

	start := time.Now()
	list, err := c.Devices(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("一个连不上的对端拖慢了整个请求: %v", elapsed)
	}
	g := list.Groups[0]
	if d := findDevice(t, g, "slow"); d.DeskBridge != nil || d.ProbeError != "连接超时" {
		t.Errorf("slow = %+v, 期望 probe_error=连接超时", d)
	}
	if d := findDevice(t, g, "fast"); d.DeskBridge == nil {
		t.Errorf("fast 应探测成功: %+v", d)
	}
}

func TestDevicesNotConnectedGroupAndSecondChannel(t *testing.T) {
	ts := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	ts.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	lan := meshtest.NewFake(mesh.KindLAN, "192.168.1.5")
	lan.SetStatus(mesh.Status{Kind: mesh.KindLAN, State: mesh.StateNeedsLogin, Reason: "未配对"})
	lan.SetPeers([]mesh.Peer{{ID: "l1", Name: "ghost", Addrs: []string{"192.168.1.9"}, Mine: true, Online: true}})
	c := client.New(startNodeChannels(t, "win-pc", ts, lan))

	list, err := c.Devices(context.Background(), false)
	if err != nil {
		t.Fatalf("有连接方式未连接时请求不应失败: %v", err)
	}
	if len(list.Groups) != 2 || list.Groups[0].Kind != mesh.KindTailscale || list.Groups[1].Kind != mesh.KindLAN {
		t.Fatalf("分组应按配置顺序: %+v", list.Groups)
	}
	g := list.Groups[1]
	if g.Status.State != mesh.StateNeedsLogin || g.Status.Reason != "未配对" {
		t.Errorf("未连接的组应返回状态: %+v", g.Status)
	}
	if g.Devices == nil || len(g.Devices) != 0 {
		t.Errorf("未连接的组 devices 应为空数组: %#v", g.Devices)
	}
}

// 新增的 GET /local/devices 与原有的 /local/devices/{target}/{rest...} 转发共存。
func TestDevicesListCoexistsWithForward(t *testing.T) {
	f := newDevicesFixture(t)
	c := client.New(f.socket)
	ctx := context.Background()

	if _, err := c.Devices(ctx, false); err != nil {
		t.Fatalf("Devices() 出错: %v", err)
	}
	// 转发到本机。
	report, err := c.Capability(ctx, "self")
	if err != nil || report.Name != "win-pc" {
		t.Errorf("Capability(self) = %+v, %v", report, err)
	}
	// 转发到对端（按名称查找）。
	report, err = c.Capability(ctx, "b-run")
	if err != nil || report.DeviceID != "dev-run" {
		t.Errorf("Capability(b-run) = %+v, %v", report, err)
	}
	// 列表接口只接受 GET。
	req, err := http.NewRequest(http.MethodPost, "http://deskbridge.local/local/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", f.socket)
	}}}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("POST /local/devices 不应成功")
	}
}

func TestMergeDuplicatesAndSort(t *testing.T) {
	info := func(id, name string) *client.DeskBridgeInfo {
		return &client.DeskBridgeInfo{DeviceID: id, DisplayName: name}
	}
	devs := []client.Device{
		{Name: "me", Self: true, Online: true, DeskBridge: info("id-me", "Me")},
		{Name: "me-copy", Online: true, Path: mesh.PathLAN, DeskBridge: info("id-me", "Me")}, // 与本机同 ID：本机优先
		{Name: "z-relay", Online: true, Path: mesh.PathRelay, DeskBridge: info("id-z", "Z")},
		{Name: "y-lan", Online: true, Path: mesh.PathLAN, DeskBridge: info("id-z", "Z")},
		{Name: "x-direct", Online: true, Path: mesh.PathDirect, DeskBridge: info("id-z", "Z")},
		{Name: "no-info", Online: true}, // 没有 DeskBridge 信息，不参与合并
		{Name: "off", Online: false},
	}
	got := sortDevices(mergeDuplicates(devs))
	var names []string
	for _, d := range got {
		names = append(names, d.Name)
	}
	want := []string{"me", "no-info", "y-lan", "off"} // 在线的按显示名排序：no-info（用短名）、Z
	if len(names) != len(want) {
		t.Fatalf("结果 = %v, 期望 %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("结果 = %v, 期望 %v", names, want)
		}
	}
	if d := got[0].Duplicates; len(d) != 1 || d[0] != "me-copy" {
		t.Errorf("本机 duplicates = %v", d)
	}
	if d := got[2].Duplicates; len(d) != 2 || d[0] != "x-direct" || d[1] != "z-relay" {
		t.Errorf("y-lan duplicates = %v, 期望 [x-direct z-relay]", d)
	}
}
