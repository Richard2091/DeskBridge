// Package meshtest 提供测试用的假连接方式，实现 mesh.Channel，
// 供节点和客户端测试使用，不访问真实网络。
package meshtest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// Fake 是假的连接方式：状态、对端列表和身份都可预设；Listen 在本机
// 127.0.0.1 上监听，记住 "假内网地址:端口 → 真实地址" 的映射，Dial 按
// 映射拨号。默认假内网地址是 100.64.0.1（Tailscale CGNAT 段的示例值）。
type Fake struct {
	mu     sync.Mutex
	kind   mesh.Kind
	fakeIP string

	status     mesh.Status
	peers      []mesh.Peer
	identities map[string]mesh.Identity // 真实远端 IP → 身份
	defaultID  *mesh.Identity           // 未预设的身份

	listeners map[string]net.Listener // 假内网地址:端口 → 真实监听
	routes    map[string]string       // 假内网地址:端口 → 真实地址（跨 Fake 连接）
	closed    bool
}

// NewFake 创建假连接方式。kind 为连接方式名，fakeIP 为本机假内网地址
// （空则用 100.64.0.1）。
func NewFake(kind mesh.Kind, fakeIP string) *Fake {
	if fakeIP == "" {
		fakeIP = "100.64.0.1"
	}
	if kind == "" {
		kind = "fake"
	}
	return &Fake{
		kind:      kind,
		fakeIP:    fakeIP,
		listeners: make(map[string]net.Listener),
		routes:    make(map[string]string),
	}
}

// Kind 返回连接方式。
func (f *Fake) Kind() mesh.Kind { return f.kind }

// Start 无实际操作。
func (f *Fake) Start(context.Context) error { return nil }

// SetStatus 设置 Status/Login 返回的状态。
func (f *Fake) SetStatus(s mesh.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = s
}

// SetPeers 设置 Peers 返回的对端列表。
func (f *Fake) SetPeers(peers []mesh.Peer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peers = peers
}

// SetIdentity 预设来自真实远端 IP（不含端口）的连接身份。
func (f *Fake) SetIdentity(remoteIP string, id mesh.Identity) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.identities == nil {
		f.identities = make(map[string]mesh.Identity)
	}
	f.identities[remoteIP] = id
}

// SetDefaultIdentity 设置未预设身份的连接的默认身份。
func (f *Fake) SetDefaultIdentity(id mesh.Identity) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defaultID = &id
}

// SetRoute 增加 "假内网地址:端口 → 真实地址:端口" 的拨号映射，
// 用于让一个 Fake 拨到另一个 Fake 所在节点。
func (f *Fake) SetRoute(fakeAddr, realAddr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[fakeAddr] = realAddr
}

// FakeIP 返回本机假内网地址。
func (f *Fake) FakeIP() string { return f.fakeIP }

// RealAddr 返回假内网地址对应的真实监听地址，没有映射时返回空。
func (f *Fake) RealAddr(fakeAddr string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ln, ok := f.listeners[fakeAddr]; ok {
		return ln.Addr().String()
	}
	return ""
}

// Status 返回预设的状态。
func (f *Fake) Status(context.Context) (mesh.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

// Snapshot 返回预设的状态和对端列表。
func (f *Fake) Snapshot(context.Context) (mesh.Status, []mesh.Peer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	peers := make([]mesh.Peer, len(f.peers))
	copy(peers, f.peers)
	return f.status, peers, nil
}

// Login 返回预设的状态（不改变它）。
func (f *Fake) Login(context.Context) (mesh.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

// Logout 把状态改成 needs_login，模拟退出登录。
func (f *Fake) Logout(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = mesh.Status{Kind: f.kind, State: mesh.StateNeedsLogin}
	return nil
}

// Peers 返回预设的对端列表。
func (f *Fake) Peers(context.Context) ([]mesh.Peer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]mesh.Peer, len(f.peers))
	copy(out, f.peers)
	return out, nil
}

// Probe 返回 Addrs 包含 addr 的对端（路径保持预设值，不测量）。
func (f *Fake) Probe(_ context.Context, addr string) (mesh.Peer, error) {
	host := addr
	if ap, err := netip.ParseAddrPort(addr); err == nil {
		host = ap.Addr().String()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.peers {
		for _, a := range p.Addrs {
			if a == host {
				return p, nil
			}
		}
	}
	return mesh.Peer{}, fmt.Errorf("%w: %s", mesh.ErrPeerNotFound, addr)
}

// Dial 按映射连接真实地址：先查本 Fake 的监听，再查 SetRoute 设置的路由。
func (f *Fake) Dial(_ context.Context, network, addr string) (net.Conn, error) {
	f.mu.Lock()
	target := ""
	if ln, ok := f.listeners[addr]; ok {
		target = ln.Addr().String()
	} else {
		target = f.routes[addr]
	}
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return nil, errors.New("meshtest: 假连接方式已关闭")
	}
	if target == "" {
		return nil, fmt.Errorf("%w: %s", mesh.ErrNotConnected, addr)
	}
	return net.Dial(network, target)
}

// Listen 在 127.0.0.1 上监听（总是用真实随机端口，避免同机多个 Fake
// 冲突），并记住 "假内网地址:端口 → 真实地址" 的映射。假地址的端口取
// addr 指定的端口（如 ":7465"），未指定时用真实端口。
func (f *Fake) Listen(network, addr string) (net.Listener, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("meshtest: 不支持的网络类型 %s", network)
	}
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return nil, errors.New("meshtest: 假连接方式已关闭")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	fakePort := ""
	if _, p, err := net.SplitHostPort(addr); err == nil && p != "" && p != "0" {
		fakePort = p
	}
	if fakePort == "" {
		_, realPort, _ := net.SplitHostPort(ln.Addr().String())
		fakePort = realPort
	}
	fakeAddr := net.JoinHostPort(f.fakeIP, fakePort)
	f.mu.Lock()
	f.listeners[fakeAddr] = ln
	f.mu.Unlock()
	return &mappedListener{Listener: ln, fakeAddr: fakeAddr}, nil
}

// WhoIs 按连接来源的真实 IP 返回预设身份，没有预设时用默认身份，
// 都没有时返回 ErrUnknownPeer。
func (f *Fake) WhoIs(_ context.Context, remoteAddr string) (mesh.Identity, error) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.identities[host]; ok {
		return id, nil
	}
	if f.defaultID != nil {
		return *f.defaultID, nil
	}
	return mesh.Identity{}, fmt.Errorf("%w: %s", mesh.ErrUnknownPeer, remoteAddr)
}

// Subscribe 返回一个永不产生事件、ctx 结束时关闭的通道。
func (f *Fake) Subscribe(ctx context.Context) <-chan mesh.Event {
	ch := make(chan mesh.Event)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

// Close 关闭所有本 Fake 打开的监听。
func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	var err error
	for _, ln := range f.listeners {
		if e := ln.Close(); e != nil && err == nil {
			err = e
		}
	}
	f.listeners = make(map[string]net.Listener)
	return err
}

// mappedListener 包装真实监听，Addr 报告假内网地址。
type mappedListener struct {
	net.Listener
	fakeAddr string
}

// Addr 返回假内网地址，让上层以为监听在内网地址上。
func (m *mappedListener) Addr() net.Addr { return fakeAddr(m.fakeAddr) }

// fakeAddr 是假内网地址的 net.Addr 实现。
type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }
