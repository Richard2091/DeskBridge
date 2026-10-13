package mesh

// Tailscale 连接方式的实现：用 tsnet 让本程序自己加入 Tailscale 内网
//（或控制服务器地址指向 Headscale 的自建网络）。

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/types/logger"
)

// TailscaleConfig 是 Tailscale 连接方式的配置。
type TailscaleConfig struct {
	Dir        string                           // tsnet 状态目录，权限 0700
	Hostname   string                           // 内网中的主机名，空则用 os.Hostname() 转小写
	ControlURL string                           // 控制服务器，空为 Tailscale 官方；改成 Headscale 地址即可使用自建
	AuthKey    string                           // 预授权密钥，空则交互式登录
	Logf       func(format string, args ...any) // tsnet 内部日志，nil 则丢弃
}

// Tailscale 实现 mesh.Channel。
type Tailscale struct {
	cfg      TailscaleConfig
	hostname string

	mu        sync.Mutex
	srv       *tsnet.Server
	lc        *local.Client
	started   bool
	closed    bool
	lastState State // 后台轮询更新的最新状态，供 Dial 快速判断
	selfID    tailcfg.UserID
	subs      map[*subscription]struct{}

	watchCtx    context.Context
	watchCancel context.CancelFunc
}

// subscription 是一个事件订阅者。
type subscription struct {
	ch   chan Event
	once sync.Once
}

func (s *subscription) close() { s.once.Do(func() { close(s.ch) }) }

// 状态轮询间隔和单次查询超时。
const (
	pollInterval  = 2 * time.Second
	statusTimeout = 10 * time.Second
	loginTimeout  = 15 * time.Second
	subBuffer     = 16
)

// NewTailscale 创建 Tailscale 连接方式，需要调用 Start 才会真正加入内网。
func NewTailscale(cfg TailscaleConfig) *Tailscale {
	hostname := cfg.Hostname
	if hostname == "" {
		if h, err := os.Hostname(); err == nil && h != "" {
			hostname = h
		} else {
			hostname = "deskbridge"
		}
	}
	return &Tailscale{
		cfg:       cfg,
		hostname:  strings.ToLower(hostname),
		lastState: StateStopped,
		subs:      make(map[*subscription]struct{}),
	}
}

// Kind 返回连接方式。
func (t *Tailscale) Kind() Kind { return KindTailscale }

// Start 启动 tsnet 并立即返回，不等登录完成。重复调用没有副作用。
func (t *Tailscale) Start(_ context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errors.New("mesh: 连接方式已关闭")
	}
	if t.started {
		t.mu.Unlock()
		return nil
	}
	logf := t.cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	srv := &tsnet.Server{
		Dir:        t.cfg.Dir,
		Hostname:   t.hostname,
		ControlURL: t.cfg.ControlURL,
		AuthKey:    t.cfg.AuthKey,
		Logf:       logger.Logf(logf),
		UserLogf:   logger.Logf(logf),
	}
	if err := srv.Start(); err != nil {
		t.mu.Unlock()
		srv.Close()
		return fmt.Errorf("启动 tsnet 失败: %w", err)
	}
	lc, err := srv.LocalClient()
	if err != nil {
		t.mu.Unlock()
		srv.Close()
		return fmt.Errorf("获取 tsnet 本机客户端失败: %w", err)
	}
	t.srv = srv
	t.lc = lc
	t.started = true
	t.watchCtx, t.watchCancel = context.WithCancel(context.Background())
	go t.watchLoop(t.watchCtx)
	t.mu.Unlock()
	return nil
}

// Status 返回当前状态。未 Start 或已 Close 时为 stopped。
func (t *Tailscale) Status(ctx context.Context) (Status, error) {
	lc, ok := t.client()
	if !ok {
		return Status{Kind: KindTailscale, State: StateStopped}, nil
	}
	sctx, cancel := withTimeout(ctx, statusTimeout)
	defer cancel()
	st, err := lc.Status(sctx)
	if err != nil {
		return Status{}, fmt.Errorf("查询 Tailscale 状态失败: %w", err)
	}
	return statusFromIPN(KindTailscale, st), nil
}

// Login 发起交互式登录。已经连接时直接返回当前状态；否则最多等待
// loginTimeout，直到出现登录链接或连接成功。
func (t *Tailscale) Login(ctx context.Context) (Status, error) {
	if err := t.Start(ctx); err != nil {
		return Status{}, err
	}
	lc, ok := t.client()
	if !ok {
		return Status{}, errors.New("mesh: 连接方式已关闭")
	}
	// 已连接时不要再发起登录，StartLoginInteractive 会作废现有会话。
	if s, err := t.Status(ctx); err == nil && s.State == StateConnected {
		return s, nil
	}
	if err := lc.StartLoginInteractive(ctx); err != nil {
		return Status{}, fmt.Errorf("发起登录失败: %w", err)
	}
	deadline := time.Now().Add(loginTimeout)
	for {
		var last Status
		sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		st, err := lc.Status(sctx)
		cancel()
		if err == nil {
			last = statusFromIPN(KindTailscale, st)
			if last.State == StateConnected || last.LoginURL != "" {
				return last, nil
			}
		}
		if ctx.Err() != nil {
			return last, ctx.Err()
		}
		if time.Now().After(deadline) {
			// 超时：把目前已知的状态带回去，错误只在没有状态时返回。
			if err != nil && last == (Status{}) {
				return Status{}, err
			}
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Logout 退出登录，之后状态为 needs_login。
func (t *Tailscale) Logout(ctx context.Context) error {
	lc, ok := t.client()
	if !ok {
		return nil // 未启动时本来就没有登录会话
	}
	lctx, cancel := withTimeout(ctx, statusTimeout)
	defer cancel()
	if err := lc.Logout(lctx); err != nil {
		return fmt.Errorf("退出登录失败: %w", err)
	}
	return nil
}

// Peers 返回所有对端，路径和延迟来自连接层已有信息。
func (t *Tailscale) Peers(ctx context.Context) ([]Peer, error) {
	st, err := t.rawStatus(ctx)
	if err != nil {
		return nil, err
	}
	return peersFromIPN(st), nil
}

// Probe 主动测量到对端（addr 为内网地址）的路径和延迟。
func (t *Tailscale) Probe(ctx context.Context, addr string) (Peer, error) {
	st, err := t.rawStatus(ctx)
	if err != nil {
		return Peer{}, err
	}
	host := addr
	if ap, err := netip.ParseAddrPort(addr); err == nil {
		host = ap.Addr().String()
	}
	var found *ipnstate.PeerStatus
	for _, p := range st.Peer {
		for _, ip := range p.TailscaleIPs {
			if ip.String() == host {
				found = p
				break
			}
		}
	}
	if found == nil {
		return Peer{}, fmt.Errorf("%w: %s", ErrPeerNotFound, addr)
	}
	ip := pingAddr(found.TailscaleIPs)
	res, err := t.lc.Ping(ctx, ip, tailcfg.PingDisco)
	if err != nil {
		return Peer{}, fmt.Errorf("探测 %s 失败: %w", addr, err)
	}
	peer := peerFromIPN(st, found)
	peer.LatencyMS = res.LatencySeconds * 1000
	if res.Endpoint != "" {
		peer.Path, peer.Endpoint, peer.Relay = classifyPath(res.Endpoint, "")
	} else if res.DERPRegionCode != "" {
		peer.Path, peer.Endpoint, peer.Relay = PathRelay, "", res.DERPRegionCode
	}
	return peer, nil
}

// Dial 经内网连接对端，addr 为 "内网地址:端口"。未连接时返回包含
// ErrNotConnected 的错误。
func (t *Tailscale) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	srv, ok := t.server()
	if !ok {
		return nil, fmt.Errorf("%w: Tailscale 未启动", ErrNotConnected)
	}
	if state := t.currentGuard(); state != StateConnected {
		return nil, fmt.Errorf("%w: Tailscale 状态为 %s", ErrNotConnected, state)
	}
	return srv.Dial(ctx, network, addr)
}

// Listen 在本机的内网地址上监听。
func (t *Tailscale) Listen(network, addr string) (net.Listener, error) {
	srv, ok := t.server()
	if !ok {
		return nil, fmt.Errorf("%w: Tailscale 未启动", ErrNotConnected)
	}
	if state := t.currentGuard(); state != StateConnected {
		return nil, fmt.Errorf("%w: Tailscale 状态为 %s", ErrNotConnected, state)
	}
	return srv.Listen(network, addr)
}

// WhoIs 根据连接的远端地址查出对方身份。
func (t *Tailscale) WhoIs(ctx context.Context, remoteAddr string) (Identity, error) {
	lc, ok := t.client()
	if !ok {
		return Identity{}, fmt.Errorf("%w: Tailscale 未启动", ErrUnknownPeer)
	}
	resp, err := lc.WhoIs(ctx, remoteAddr)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrUnknownPeer, err)
	}
	id := Identity{
		Kind:   KindTailscale,
		Device: resp.Node.ComputedName,
		PeerID: string(resp.Node.StableID),
	}
	if resp.UserProfile != nil {
		id.Owner = resp.UserProfile.LoginName
		id.OwnerName = resp.UserProfile.DisplayName
	}
	t.mu.Lock()
	selfID := t.selfID
	t.mu.Unlock()
	if selfID == 0 {
		// 后台轮询还没取到本机账号（刚启动），当场查一次，避免误拒同账号请求。
		if st, err := lc.Status(ctx); err == nil && st.Self != nil {
			selfID = st.Self.UserID
		}
	}
	id.Mine = selfID != 0 && resp.Node.User == selfID && len(resp.Node.Tags) == 0
	return id, nil
}

// Subscribe 返回事件通道，ctx 结束时关闭。通道带缓冲，满了丢弃新事件。
func (t *Tailscale) Subscribe(ctx context.Context) <-chan Event {
	sub := &subscription{ch: make(chan Event, subBuffer)}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		close(sub.ch)
		return sub.ch
	}
	t.subs[sub] = struct{}{}
	t.mu.Unlock()
	go func() {
		<-ctx.Done()
		t.mu.Lock()
		delete(t.subs, sub)
		t.mu.Unlock()
		sub.close()
	}()
	return sub.ch
}

// Close 关闭 tsnet、停止后台轮询并关闭所有订阅通道。登录状态保留，
// 下次 Start 自动恢复。
func (t *Tailscale) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	cancel := t.watchCancel
	srv := t.srv
	subs := t.subs
	t.subs = make(map[*subscription]struct{})
	t.srv = nil
	t.lc = nil
	t.started = false
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if srv != nil {
		srv.Close()
	}
	for sub := range subs {
		sub.close()
	}
	return nil
}

// client 返回本机客户端；未启动或已关闭时 ok 为 false。
func (t *Tailscale) client() (*local.Client, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lc, t.started && !t.closed && t.lc != nil
}

// server 返回 tsnet 服务；未启动或已关闭时 ok 为 false。
func (t *Tailscale) server() (*tsnet.Server, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.srv, t.started && !t.closed && t.srv != nil
}

// currentGuard 返回最近一次轮询到的状态，用于 Dial/Listen 前的快速判断。
func (t *Tailscale) currentGuard() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastState
}

// rawStatus 查询原始 tsnet 状态；未启动时返回空状态而不是错误，
// 让 Peers 在未连接时返回空列表。
func (t *Tailscale) rawStatus(ctx context.Context) (*ipnstate.Status, error) {
	lc, ok := t.client()
	if !ok {
		return nil, nil
	}
	sctx, cancel := withTimeout(ctx, statusTimeout)
	defer cancel()
	st, err := lc.Status(sctx)
	if err != nil {
		return nil, fmt.Errorf("查询 Tailscale 状态失败: %w", err)
	}
	return st, nil
}

// watchLoop 每 2 秒读取一次 tsnet 状态，对比前后差异产生事件并分发。
func (t *Tailscale) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var last *snapshot
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		t.mu.Lock()
		lc := t.lc
		t.mu.Unlock()
		if lc == nil {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, statusTimeout)
		st, err := lc.Status(sctx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		snap := newSnapshot(KindTailscale, st)
		t.mu.Lock()
		t.lastState = snap.status.State
		if st.Self != nil {
			t.selfID = st.Self.UserID
		}
		t.mu.Unlock()
		for _, ev := range diffSnapshots(last, snap) {
			t.dispatch(ev)
		}
		last = snap
	}
}

// dispatch 把事件发给所有订阅者，通道满时丢弃。
func (t *Tailscale) dispatch(ev Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for sub := range t.subs {
		select {
		case sub.ch <- ev:
		default:
		}
	}
}

// snapshot 是一次状态读取的转换结果，用于对比差异。
type snapshot struct {
	status Status
	peers  map[string]Peer
}

func newSnapshot(kind Kind, st *ipnstate.Status) *snapshot {
	s := &snapshot{status: statusFromIPN(kind, st), peers: make(map[string]Peer, len(st.Peer))}
	for _, p := range st.Peer {
		peer := peerFromIPN(st, p)
		s.peers[peer.ID] = peer
	}
	return s
}

// diffSnapshots 对比两次快照产生事件。首次快照与"未启动"比较，只报状态变化。
func diffSnapshots(old, new *snapshot) []Event {
	if old == nil {
		if new.status.State != StateStopped {
			return []Event{{Type: EventState, Time: time.Now(), Status: &new.status}}
		}
		return nil
	}
	var evs []Event
	add := func(t EventType, peer Peer) {
		p := peer
		evs = append(evs, Event{Type: t, Time: time.Now(), Peer: &p})
	}
	if old.status.State != new.status.State {
		s := new.status
		evs = append(evs, Event{Type: EventState, Time: time.Now(), Status: &s})
	}
	for id, np := range new.peers {
		op, ok := old.peers[id]
		if !ok {
			if np.Online {
				add(EventPeerOnline, np)
			}
			continue
		}
		switch {
		case np.Online && !op.Online:
			add(EventPeerOnline, np)
		case !np.Online && op.Online:
			add(EventPeerOffline, np)
		case np.Path != op.Path || np.Relay != op.Relay || np.Endpoint != op.Endpoint:
			add(EventPeerPath, np)
		}
	}
	for id, op := range old.peers {
		if _, ok := new.peers[id]; !ok {
			add(EventPeerOffline, op)
		}
	}
	return evs
}

// pingAddr 选出用于探测的 IP：优先 IPv4。
func pingAddr(ips []netip.Addr) netip.Addr {
	var v6 netip.Addr
	for _, ip := range ips {
		if ip.Is4() {
			return ip
		}
		if !v6.IsValid() {
			v6 = ip
		}
	}
	return v6
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d)
}
