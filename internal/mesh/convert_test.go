package mesh

// 状态转换、对端转换、路径判断纯函数的单元测试，全部用构造的
// ipnstate.Status，不依赖网络。

import (
	"bytes"
	"net/netip"
	"testing"
	"time"

	"go4.org/mem"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

func statusForTest(self *ipnstate.PeerStatus, peers ...*ipnstate.PeerStatus) *ipnstate.Status {
	st := &ipnstate.Status{
		BackendState: "Running",
		Self:         self,
		Peer:         map[key.NodePublic]*ipnstate.PeerStatus{},
		User:         map[tailcfg.UserID]tailcfg.UserProfile{},
	}
	if self != nil {
		st.User[self.UserID] = tailcfg.UserProfile{LoginName: "alice@example.com", DisplayName: "Alice"}
	}
	for i, p := range peers {
		st.Peer[key.NodePublicFromRaw32(mem.B(bytes.Repeat([]byte{byte(i + 1)}, 32)))] = p
		st.User[p.UserID] = tailcfg.UserProfile{LoginName: "alice@example.com"}
	}
	return st
}

func TestStatusFromIPNStates(t *testing.T) {
	for _, c := range []struct {
		backend string
		want    State
	}{
		{"NeedsLogin", StateNeedsLogin},
		{"NeedsMachineAuth", StateNeedsLogin},
		{"Running", StateConnected},
		{"Starting", StateStarting},
		{"NoState", StateStarting},
		{"Stopped", StateDisconnected},
		{"", StateStarting},
	} {
		st := statusForTest(nil)
		st.BackendState = c.backend
		got := statusFromIPN(KindTailscale, st)
		if got.State != c.want {
			t.Errorf("BackendState %q → %s, 期望 %s", c.backend, got.State, c.want)
		}
		if got.Kind != KindTailscale {
			t.Errorf("Kind = %s, 期望 tailscale", got.Kind)
		}
	}
}

func TestStatusFromIPNNeedsMachineAuthReason(t *testing.T) {
	st := statusForTest(nil)
	st.BackendState = "NeedsMachineAuth"
	got := statusFromIPN(KindTailscale, st)
	if got.Reason != "等待管理员在控制台批准此设备" {
		t.Errorf("Reason = %q", got.Reason)
	}
}

func TestStatusFromIPNLoginURL(t *testing.T) {
	st := statusForTest(nil)
	st.BackendState = "NeedsLogin"
	st.AuthURL = "https://login.tailscale.com/a/0123456789"
	got := statusFromIPN(KindTailscale, st)
	if got.LoginURL != st.AuthURL {
		t.Errorf("LoginURL = %q, 期望 %q", got.LoginURL, st.AuthURL)
	}
}

func TestStatusFromIPNAccountAndSelf(t *testing.T) {
	self := &ipnstate.PeerStatus{
		ID:           "self-id",
		HostName:     "win-pc",
		DNSName:      "win-pc.tailnet-1234.ts.net.",
		UserID:       1,
		TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")},
	}
	st := statusForTest(self)
	got := statusFromIPN(KindTailscale, st)
	if got.Account != "alice@example.com" {
		t.Errorf("Account = %q", got.Account)
	}
	if got.Self == nil || got.Self.Name != "win-pc" || got.Self.ID != "self-id" {
		t.Errorf("Self 转换错误: %+v", got.Self)
	}
}

func ipSlice(addrs ...string) []netip.Addr {
	var out []netip.Addr
	for _, a := range addrs {
		out = append(out, netip.MustParseAddr(a))
	}
	return out
}

func TestPeerFromIPN(t *testing.T) {
	st := statusForTest(
		&ipnstate.PeerStatus{UserID: 1},
		&ipnstate.PeerStatus{
			ID:           "peer-1",
			HostName:     "Linux-Box",
			DNSName:      "linux-box.tailnet-1234.ts.net.",
			OS:           "linux",
			UserID:       1,
			TailscaleIPs: ipSlice("fd7a:115c:a1e0::1", "100.64.0.2"),
			Online:       true,
			CurAddr:      "203.0.113.7:41641",
			LastSeen:     time.Unix(1700000000, 0).UTC(),
		},
	)
	peers := peersFromIPN(st)
	if len(peers) != 1 {
		t.Fatalf("对端数量 = %d, 期望 1", len(peers))
	}
	p := peers[0]
	if p.ID != "peer-1" || p.Name != "linux-box" || p.HostName != "Linux-Box" {
		t.Errorf("ID/Name/HostName = %q/%q/%q", p.ID, p.Name, p.HostName)
	}
	if p.OS != "linux" || !p.Online || !p.Mine || p.Owner != "alice@example.com" {
		t.Errorf("OS/Online/Mine/Owner = %q/%v/%v/%q", p.OS, p.Online, p.Mine, p.Owner)
	}
	// IPv4 在前。
	if len(p.Addrs) != 2 || p.Addrs[0] != "100.64.0.2" || p.Addrs[1] != "fd7a:115c:a1e0::1" {
		t.Errorf("Addrs = %v, 期望 IPv4 在前", p.Addrs)
	}
	if p.Path != PathDirect || p.Endpoint != "203.0.113.7:41641" {
		t.Errorf("Path/Endpoint = %s/%q", p.Path, p.Endpoint)
	}
}

func TestPeerNameFallsBackToHostName(t *testing.T) {
	st := statusForTest(
		&ipnstate.PeerStatus{UserID: 1},
		&ipnstate.PeerStatus{ID: "p", HostName: "MacBook-Pro", UserID: 1},
	)
	p := peersFromIPN(st)[0]
	if p.Name != "macbook-pro" {
		t.Errorf("Name = %q, 期望 macbook-pro", p.Name)
	}
}

// ptr 返回 v 的地址，方便在构造字面量里使用。
func ptr[T any](v T) *T { return &v }

func TestPeerMineFalseForOtherUserAndTags(t *testing.T) {
	self := &ipnstate.PeerStatus{UserID: 1}
	other := &ipnstate.PeerStatus{ID: "other", UserID: 2, Online: true}
	tagged := &ipnstate.PeerStatus{
		ID:     "tagged",
		UserID: 0,
		Tags:   ptr(views.SliceOf([]string{"tag:server"})),
	}
	st := statusForTest(self, other, tagged)
	peers := peersFromIPN(st)
	for _, p := range peers {
		if p.Mine {
			t.Errorf("对端 %s 不应标记为 Mine", p.ID)
		}
	}
}

func TestDNSShortName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"win-pc.tailnet-1234.ts.net.", "win-pc"},
		{"host.example.com", "host"},
		{"only", "only"},
		{"", ""},
	} {
		if got := dnsShortName(c.in); got != c.want {
			t.Errorf("dnsShortName(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestClassifyPath(t *testing.T) {
	for _, c := range []struct {
		cur, relay string
		wantPath   PathKind
		wantEnd    string
		wantRelay  string
	}{
		{"192.168.1.5:41641", "", PathLAN, "192.168.1.5:41641", ""},
		{"10.0.0.2:1", "", PathLAN, "10.0.0.2:1", ""},
		{"172.16.0.2:1", "", PathLAN, "172.16.0.2:1", ""},
		{"169.254.1.2:1", "", PathLAN, "169.254.1.2:1", ""},
		{"fd00::5:1", "", PathLAN, "fd00::5:1", ""},
		{"fe80::5:1", "", PathLAN, "fe80::5:1", ""},
		{"203.0.113.7:41641", "", PathDirect, "203.0.113.7:41641", ""},
		{"", "hkg", PathRelay, "", "hkg"},
		{"", "", PathUnknown, "", ""},
	} {
		path, ep, relay := classifyPath(c.cur, c.relay)
		if path != c.wantPath || ep != c.wantEnd || relay != c.wantRelay {
			t.Errorf("classifyPath(%q, %q) = (%s, %q, %q), 期望 (%s, %q, %q)",
				c.cur, c.relay, path, ep, relay, c.wantPath, c.wantEnd, c.wantRelay)
		}
	}
}

func TestDiffSnapshotsFirstStateEvent(t *testing.T) {
	st := statusForTest(nil)
	st.BackendState = "NeedsLogin"
	snap := newSnapshot(KindTailscale, st)
	evs := diffSnapshots(nil, snap)
	if len(evs) != 1 || evs[0].Type != EventState {
		t.Fatalf("首轮应只产生 1 个状态事件, 得到 %d 个", len(evs))
	}
	if evs[0].Status == nil || evs[0].Status.State != StateNeedsLogin {
		t.Errorf("事件状态错误: %+v", evs[0].Status)
	}
}

func TestDiffSnapshotsPeerEvents(t *testing.T) {
	// 构造两组快照：上线 → 路径变化 → 下线。
	peerOnline := func(cur string) *ipnstate.Status {
		return statusForTest(nil, &ipnstate.PeerStatus{ID: "p1", HostName: "peer", Online: true, CurAddr: cur})
	}
	first := newSnapshot(KindTailscale, peerOnline(""))
	diffSnapshots(nil, first) // 首轮只建立基准

	// 上线后有了直连地址 → 路径事件。
	second := newSnapshot(KindTailscale, peerOnline("203.0.113.9:100"))
	evs := diffSnapshots(first, second)
	var hasPath bool
	for _, ev := range evs {
		if ev.Type == EventPeerPath {
			hasPath = true
		}
	}
	if !hasPath {
		t.Errorf("路径变化应产生 EventPeerPath, 得到 %+v", evs)
	}

	// 对端从列表消失 → 下线事件。
	third := newSnapshot(KindTailscale, statusForTest(nil))
	evs = diffSnapshots(second, third)
	var hasOffline bool
	for _, ev := range evs {
		if ev.Type == EventPeerOffline && ev.Peer != nil && ev.Peer.ID == "p1" {
			hasOffline = true
		}
	}
	if !hasOffline {
		t.Errorf("对端消失应产生 EventPeerOffline, 得到 %+v", evs)
	}

	// 新对端在线 → 上线事件。
	fourth := newSnapshot(KindTailscale, peerOnline(""))
	fourth.peers["new"] = Peer{ID: "new", Online: true}
	evs = diffSnapshots(third, fourth)
	var hasOnline bool
	for _, ev := range evs {
		if ev.Type == EventPeerOnline && ev.Peer != nil && ev.Peer.ID == "new" {
			hasOnline = true
		}
	}
	if !hasOnline {
		t.Errorf("新对端应产生 EventPeerOnline, 得到 %+v", evs)
	}
}
