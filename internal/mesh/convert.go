package mesh

// 状态转换、对端转换、路径判断的纯函数，不依赖网络，可单独测试。

import (
	"net/netip"
	"sort"
	"strings"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/views"
)

// statusFromIPN 把 tsnet 本机状态转换成连接层状态。
// backendState 为空（例如 tsnet 未启动）时视为 stopped。
func statusFromIPN(kind Kind, st *ipnstate.Status) Status {
	s := Status{Kind: kind, State: StateStopped}
	if st == nil {
		return s
	}
	switch st.BackendState {
	case "NeedsLogin":
		s.State = StateNeedsLogin
		s.LoginURL = st.AuthURL
	case "NeedsMachineAuth":
		s.State = StateNeedsLogin
		s.Reason = "等待管理员在控制台批准此设备"
		if st.AuthURL != "" {
			s.LoginURL = st.AuthURL
		}
	case "Running":
		s.State = StateConnected
	case "Starting", "NoState":
		s.State = StateStarting
	case "Stopped":
		s.State = StateDisconnected
	default:
		s.State = StateStarting
	}
	if st.Self != nil {
		self := peerFromIPN(st, st.Self)
		s.Self = &self
		if u, ok := st.User[st.Self.UserID]; ok {
			s.Account = u.LoginName
		}
	}
	return s
}

// peersFromIPN 把 tsnet 状态中的所有对端转换成连接层对端，按名称排序。
func peersFromIPN(st *ipnstate.Status) []Peer {
	if st == nil {
		return nil
	}
	peers := make([]Peer, 0, len(st.Peer))
	for _, p := range st.Peer {
		peers = append(peers, peerFromIPN(st, p))
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].Name < peers[j].Name })
	return peers
}

// peerFromIPN 把 tsnet 的一个对端转换成连接层对端。
func peerFromIPN(st *ipnstate.Status, p *ipnstate.PeerStatus) Peer {
	peer := Peer{
		ID:       string(p.ID),
		HostName: p.HostName,
		OS:       p.OS,
		Online:   p.Online,
		LastSeen: p.LastSeen,
		Addrs:    addrsV4First(p.TailscaleIPs),
	}
	peer.Name = dnsShortName(p.DNSName)
	if peer.Name == "" {
		peer.Name = strings.ToLower(p.HostName)
	}
	if st != nil {
		if u, ok := st.User[p.UserID]; ok {
			peer.Owner = u.LoginName
		}
		peer.Mine = sameOwner(st, p.UserID, p.Tags)
	}
	peer.Path, peer.Endpoint, peer.Relay = classifyPath(p.CurAddr, p.Relay)
	return peer
}

// sameOwner 判断一个节点是否与本地同一账号：UserID 相同且非零，双方都没有 tag。
// 带 tag 的节点归 tailnet 所有，不再属于个人账号。
func sameOwner(st *ipnstate.Status, userID tailcfg.UserID, tags *views.Slice[string]) bool {
	if st == nil || st.Self == nil || userID == 0 || userID != st.Self.UserID {
		return false
	}
	if tags != nil && tags.Len() > 0 {
		return false
	}
	if self := st.Self; self.Tags != nil && self.Tags.Len() > 0 {
		return false
	}
	return true
}

// dnsShortName 取 DNS 名的第一段（去掉末尾的点），如 "win-pc.tailnet-xxxx.ts.net."
// → "win-pc"。空字符串返回空。
func dnsShortName(dns string) string {
	dns = strings.TrimSuffix(dns, ".")
	if i := strings.Index(dns, "."); i >= 0 {
		dns = dns[:i]
	}
	return dns
}

// addrsV4First 把内网地址转成字符串，IPv4 在前。
func addrsV4First(ips []netip.Addr) []string {
	if len(ips) == 0 {
		return nil
	}
	var v4, v6 []string
	for _, ip := range ips {
		if ip.Is4() {
			v4 = append(v4, ip.String())
		} else {
			v6 = append(v6, ip.String())
		}
	}
	return append(v4, v6...)
}

// classifyPath 判断路径类型：
//   - CurAddr 非空：IP 是私有地址（10/8、172.16/12、192.168/16、169.254/16、
//     fc00::/7、fe80::/10）为局域网直连，否则跨网直连，Endpoint = CurAddr；
//   - CurAddr 为空且 Relay 非空：中继，Relay = 区域代码；
//   - 都为空：unknown。
func classifyPath(curAddr, relay string) (path PathKind, endpoint, relayCode string) {
	if curAddr != "" {
		if isPrivateIP(addrFromEndpoint(curAddr)) {
			return PathLAN, curAddr, ""
		}
		return PathDirect, curAddr, ""
	}
	if relay != "" {
		return PathRelay, "", relay
	}
	return PathUnknown, "", ""
}

// addrFromEndpoint 从 "IP:端口"（或裸 IP）中解析出 IP，解析失败返回零值。
func addrFromEndpoint(ep string) netip.Addr {
	if ap, err := netip.ParseAddrPort(ep); err == nil {
		return ap.Addr()
	}
	a, _ := netip.ParseAddr(ep)
	return a
}

// isPrivateIP 判断 IP 是否为私有或链路本地地址。
func isPrivateIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	ip = ip.Unmap()
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
