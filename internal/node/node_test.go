package node

// 用 meshtest.Fake 测节点：内网协议、本机接口和转发，不访问真实网络。
// unix socket 放在 os.MkdirTemp 建的短路径目录里（Windows 上 t.TempDir()
// 可能超过 AF_UNIX 的 108 字符限制）。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/deviceid"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/mesh/meshtest"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/store"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// shortTempDir 返回短路径的临时目录，socket 文件放这里。
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// testDevice 返回一个测试设备 ID（用文件存储，不碰密钥库）。
func testDevice(t *testing.T) *deviceid.Device {
	t.Helper()
	d, err := deviceid.Load(&deviceid.FileStore{Path: filepath.Join(shortTempDir(t), "device.key")})
	if err != nil {
		t.Fatalf("加载设备 ID 失败: %v", err)
	}
	return d
}

// testTools 返回契约测试用的工具集：test.echo 回显参数，test.sleep 睡眠。
func testTools(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	mustRegister(t, reg, echoTool())
	mustRegister(t, reg, sleepTool())
	return reg
}

// mustRegister 注册工具，失败时终止测试。
func mustRegister(t *testing.T, reg *tools.Registry, tool *tools.Tool) {
	t.Helper()
	if err := reg.Register(tool); err != nil {
		t.Fatalf("注册工具 %s 失败: %v", tool.Spec.Name, err)
	}
}

// echoTool 是回显参数的测试工具。
func echoTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "test.echo",
			Description: "原样返回参数里的文本",
			Source:      protocol.SourceBuiltin,
		},
		Run: func(ctx context.Context, call *tools.Call) (any, error) {
			var args struct {
				Text string `json:"text"`
			}
			if err := tools.DecodeArgs(call.Args, &args); err != nil {
				return nil, err
			}
			return map[string]any{"text": args.Text}, nil
		},
	}
}

// sleepTool 是按指定毫秒数睡眠的测试工具，用于超时和取消。
func sleepTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "test.sleep",
			Description: "睡眠指定的毫秒数",
			Source:      protocol.SourceBuiltin,
		},
		DefaultTimeout: 10 * time.Second,
		MaxTimeout:     time.Minute,
		Run: func(ctx context.Context, call *tools.Call) (any, error) {
			var args struct {
				MS int `json:"ms"`
			}
			if err := tools.DecodeArgs(call.Args, &args); err != nil {
				return nil, err
			}
			select {
			case <-time.After(time.Duration(args.MS) * time.Millisecond):
				return map[string]any{"slept_ms": args.MS}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
}

// startNode 启动一个带假连接方式的节点（Fake 已置为 connected，
// 节点会立即在内网监听），返回 Fake 和 socket 路径。
func startNode(t *testing.T, name string) (*meshtest.Fake, string) {
	t.Helper()
	return startNodeWithTools(t, name, nil)
}

// startNodeWithTools 同 startNode，但给节点挂上工具注册表（nil 为无工具）。
func startNodeWithTools(t *testing.T, name string, reg *tools.Registry) (*meshtest.Fake, string) {
	t.Helper()
	return startNodeWithRecords(t, name, reg, nil)
}

// startNodeWithRecords 同 startNodeWithTools，并让节点把操作记录写进 db（nil 为不记录）。
func startNodeWithRecords(t *testing.T, name string, reg *tools.Registry, db *store.DB) (*meshtest.Fake, string) {
	t.Helper()
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	socket := filepath.Join(shortTempDir(t), name+".sock")
	n := New(Config{
		Channels: []mesh.Channel{fake},
		Device:   testDevice(t),
		Tools:    reg,
		Records:  db,
		Name:     name,
		Socket:   socket,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("节点没有在 5 秒内退出")
		}
	})
	waitFor(t, func() bool {
		_, err := os.Stat(socket)
		return err == nil
	}, "节点没有在超时内监听 socket")
	return fake, socket
}

// waitFor 反复检查条件直到满足或超时。
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}

// waitChannelListening 等节点在 Fake 的标准端口上监听，返回真实监听地址。
func waitChannelListening(t *testing.T, fake *meshtest.Fake) string {
	t.Helper()
	var real string
	waitFor(t, func() bool {
		real = fake.RealAddr("100.64.0.1:7465")
		return real != ""
	}, "节点没有在 Fake 上监听端口 7465")
	return real
}

// mineIdentity 是同一账号设备的身份。
func mineIdentity(name string) mesh.Identity {
	return mesh.Identity{Kind: mesh.KindTailscale, Owner: "alice@example.com", Device: name, PeerID: "peer-1", Mine: true}
}

func TestCapabilityMineAllowed(t *testing.T) {
	fake, _ := startNode(t, "win-pc")
	fake.SetDefaultIdentity(mineIdentity("linux-box"))
	real := waitChannelListening(t, fake)

	resp, err := http.Get("http://" + real + "/v1/capability")
	if err != nil {
		t.Fatalf("请求 capability 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("状态码 = %d, 期望 200（%s）", resp.StatusCode, body)
	}
	var cap protocol.Capability
	if err := json.NewDecoder(resp.Body).Decode(&cap); err != nil {
		t.Fatalf("解析 capability 失败: %v", err)
	}
	if cap.Protocol != protocol.Version || cap.Name != "win-pc" {
		t.Errorf("capability 内容不对: %+v", cap)
	}
	if cap.Platform.OS == "" || cap.Platform.Arch == "" {
		t.Errorf("platform 缺失: %+v", cap.Platform)
	}
	if cap.Tools == nil {
		t.Error("tools 应为空数组而不是 null")
	}
}

func TestCapabilityNonMineForbidden(t *testing.T) {
	fake, _ := startNode(t, "win-pc")
	fake.SetDefaultIdentity(mesh.Identity{Kind: mesh.KindTailscale, Owner: "mallory@example.com", Device: "evil", Mine: false})
	real := waitChannelListening(t, fake)

	resp, err := http.Get("http://" + real + "/v1/capability")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("状态码 = %d, 期望 403", resp.StatusCode)
	}
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		t.Fatalf("解析错误响应失败: %v", err)
	}
	if e.Error.Code != "permission_denied" {
		t.Errorf("错误码 = %q, 期望 permission_denied", e.Error.Code)
	}
}

func TestLocalStatusAPI(t *testing.T) {
	fake, socket := startNode(t, "win-pc")
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected, Account: "alice@example.com"})

	st, err := client.New(socket).Status(context.Background())
	if err != nil {
		t.Fatalf("Status() 出错: %v", err)
	}
	if st.Name != "win-pc" || st.Version == "" || st.DeviceID == "" {
		t.Errorf("状态内容不对: %+v", st)
	}
	if len(st.Channels) != 1 || st.Channels[0].State != mesh.StateConnected || st.Channels[0].Kind != mesh.KindTailscale {
		t.Errorf("连接方式状态不对: %+v", st.Channels)
	}
}

func TestNodeAlreadyRunning(t *testing.T) {
	_, socket := startNode(t, "win-pc")
	n := New(Config{
		Channels: []mesh.Channel{meshtest.NewFake(mesh.KindTailscale, "")},
		Device:   testDevice(t),
		Name:     "other",
		Socket:   socket,
	})
	err := n.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "节点已在运行") {
		t.Errorf("Run() = %v, 期望提示节点已在运行", err)
	}
}

func TestOnListening(t *testing.T) {
	_, socket := startNode(t, "win-pc")
	called := false
	n := New(Config{
		Channels:    []mesh.Channel{meshtest.NewFake(mesh.KindTailscale, "")},
		Device:      testDevice(t),
		Name:        "other",
		Socket:      socket,
		OnListening: func() { called = true },
	})
	if err := n.Run(context.Background()); err == nil {
		t.Fatal("socket 已被占用时 Run 应失败")
	}
	if called {
		t.Error("没有监听成功时不应调用 OnListening")
	}

	ready := make(chan struct{})
	n = New(Config{
		Channels:    []mesh.Channel{meshtest.NewFake(mesh.KindTailscale, "")},
		Device:      testDevice(t),
		Name:        "fresh",
		Socket:      filepath.Join(shortTempDir(t), "fresh.sock"),
		OnListening: func() { close(ready) },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = n.Run(ctx) }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("监听成功后没有调用 OnListening")
	}
}

// startRelayNode 启动第二个节点，它的 Fake 里有指向 targetFake 的路由，
// 返回它的 socket 路径。
func startRelayNode(t *testing.T, targetFake *meshtest.Fake, peers []mesh.Peer, socketName string) string {
	fakeB := meshtest.NewFake(mesh.KindTailscale, "100.64.0.2")
	fakeB.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	fakeB.SetPeers(peers)
	waitFor(t, func() bool {
		real := targetFake.RealAddr("100.64.0.1:7465")
		if real == "" {
			return false
		}
		fakeB.SetRoute("100.64.0.1:7465", real)
		return true
	}, "目标节点没有在 Fake 上监听端口 7465")
	_, socket := startNodeWithFake(t, fakeB, "node-b", socketName)
	return socket
}

// startNodeWithFake 用现成的 Fake 启动节点。
func startNodeWithFake(t *testing.T, fake *meshtest.Fake, name, socketName string) (*meshtest.Fake, string) {
	t.Helper()
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	socket := filepath.Join(shortTempDir(t), socketName+".sock")
	n := New(Config{
		Channels: []mesh.Channel{fake},
		Device:   testDevice(t),
		Name:     name,
		Socket:   socket,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = n.Run(ctx) }()
	waitFor(t, func() bool {
		_, err := os.Stat(socket)
		return err == nil
	}, "节点没有在超时内监听 socket")
	return fake, socket
}

func TestForwardBetweenNodes(t *testing.T) {
	// 节点 A（被访问方）和节点 B（发起方）。
	fakeA, _ := startNode(t, "node-a")
	fakeA.SetDefaultIdentity(mineIdentity("node-b"))

	socketB := startRelayNode(t, fakeA, []mesh.Peer{{
		ID:    "peer-a",
		Name:  "node-a",
		Addrs: []string{"100.64.0.1"},
		Mine:  true,
	}}, "node-b")

	data, err := client.New(socketB).Get(context.Background(), "node-a", "/v1/capability")
	if err != nil {
		t.Fatalf("转发失败: %v", err)
	}
	var cap protocol.Capability
	if err := json.Unmarshal(data, &cap); err != nil {
		t.Fatalf("解析 capability 失败: %v（%s）", err, data)
	}
	if cap.Name != "node-a" {
		t.Errorf("capability.Name = %q, 期望 node-a", cap.Name)
	}
}

func TestForwardKindQualifiedTarget(t *testing.T) {
	fakeA, _ := startNode(t, "node-a")
	fakeA.SetDefaultIdentity(mineIdentity("node-b"))

	socketB := startRelayNode(t, fakeA, []mesh.Peer{{
		ID:    "peer-a",
		Name:  "Node-A", // 大小写不同的名称也应匹配
		Addrs: []string{"100.64.0.1"},
		Mine:  true,
	}}, "kb")

	data, err := client.New(socketB).Get(context.Background(), "tailscale:node-a", "/v1/capability")
	if err != nil {
		t.Fatalf("转发失败: %v", err)
	}
	if !strings.Contains(string(data), "node-a") {
		t.Errorf("响应不包含 node-a: %s", data)
	}
}

func TestForwardTargetNotFound(t *testing.T) {
	_, socket := startNode(t, "node-b")

	_, err := client.New(socket).Get(context.Background(), "ghost", "/v1/capability")
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != "device_not_found" {
		t.Errorf("Get() = %v, 期望 device_not_found", err)
	}
}

func TestForwardUnavailable(t *testing.T) {
	// 对端在列表里但拨不到（没有路由）。
	_, socket := startNodeWithFake(t, func() *meshtest.Fake {
		f := meshtest.NewFake(mesh.KindTailscale, "100.64.0.2")
		f.SetPeers([]mesh.Peer{{ID: "p", Name: "far", Addrs: []string{"100.64.0.9"}, Mine: true}})
		return f
	}(), "node-b", "fwd")

	_, err := client.New(socket).Get(context.Background(), "far", "/v1/capability")
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != "unavailable" {
		t.Errorf("Get() = %v, 期望 unavailable", err)
	}
}

func TestLoginLogoutViaLocalAPI(t *testing.T) {
	fake, socket := startNode(t, "win-pc")
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateNeedsLogin, LoginURL: "https://example.com/login"})

	c := client.New(socket)
	st, err := c.Login(context.Background(), "tailscale")
	if err != nil {
		t.Fatalf("Login() 出错: %v", err)
	}
	if st.LoginURL != "https://example.com/login" {
		t.Errorf("LoginURL = %q", st.LoginURL)
	}
	if err := c.Logout(context.Background(), "tailscale"); err != nil {
		t.Fatalf("Logout() 出错: %v", err)
	}
	// Fake.Logout 把状态改成 needs_login。
	st2, _ := fake.Status(context.Background())
	if st2.State != mesh.StateNeedsLogin {
		t.Errorf("Logout 后状态 = %s", st2.State)
	}
	// 未知连接方式 → unknown_channel。
	if _, err := c.Login(context.Background(), "lan"); err == nil {
		t.Error("未知连接方式的 Login() 应报错")
	} else {
		var perr *protocol.Error
		if !errors.As(err, &perr) || perr.Code != "unknown_channel" {
			t.Errorf("错误 = %v, 期望 unknown_channel", err)
		}
	}
}

func TestLockFileSingleInstance(t *testing.T) {
	lock := filepath.Join(shortTempDir(t), "node.lock")
	mk := func(sock string) *Node {
		return New(Config{
			Channels: []mesh.Channel{meshtest.NewFake(mesh.KindTailscale, "")},
			Device:   testDevice(t),
			Name:     "n",
			Socket:   filepath.Join(shortTempDir(t), sock),
			LockFile: lock,
		})
	}
	first := mk("a.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- first.Run(ctx) }()
	waitFor(t, func() bool {
		_, err := os.Stat(first.cfg.Socket)
		return err == nil
	}, "第一个节点没有监听 socket")

	// 不同 socket、同一把锁：第二个节点必须拒绝启动，且不碰第一个节点的 socket。
	err := mk("b.sock").Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "节点已在运行") {
		t.Fatalf("Run() = %v, 期望提示节点已在运行", err)
	}
	if _, serr := os.Stat(first.cfg.Socket); serr != nil {
		t.Errorf("第一个节点的 socket 不应受影响: %v", serr)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("第一个节点没有退出")
	}
	// 锁在 Run 返回时释放：新节点可以接着启动。
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	third := mk("c.sock")
	done2 := make(chan error, 1)
	go func() { done2 <- third.Run(ctx2) }()
	waitFor(t, func() bool {
		_, err := os.Stat(third.cfg.Socket)
		return err == nil
	}, "锁释放后新节点没有启动")
	cancel2()
	<-done2
}
