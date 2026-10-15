package node

// 目标解析：把命令行或客户端给出的设备写法（self、名称、显示名称、主机名、
// IP，可带 "连接方式:" 前缀）解析成本机或某个连接方式里的对端。

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// 匹配级别，数值越小优先级越高（self 关键字在这之前单独处理）。
const (
	matchName        = iota // 内网短名（mesh.Peer.Name）
	matchDisplayName        // DeskBridge 设备名称
	matchHostName           // 系统主机名
	matchAddr               // 内网 IP
)

// resolved 是目标解析的结果。
type resolved struct {
	self bool         // 目标是本机，不经过内网
	ch   mesh.Channel // 对端所在的连接方式；本机时为 self 匹配到的连接方式或 nil
	peer mesh.Peer    // 对端；本机时是本机信息（至少含名称）
}

// candidate 是一个匹配上的候选。
type candidate struct {
	resolved
	level int
}

// resolveTarget 解析 target。找不到时 ok 为 false。每个连接方式最多查询
// 一次状态；DeskBridge 显示名称取自探测缓存，连接层和缓存都匹配不到时，
// 才对在线的同账号对端探测一次再匹配。
//
// 优先级：self 关键字 > 名称 > DeskBridge 显示名称 > 主机名 > IP。
// 同级多个候选时：本机、同账号、在线、路径好、名称字典序。
func (n *Node) resolveTarget(ctx context.Context, target string) (resolved, bool) {
	if target == "self" {
		return resolved{self: true, peer: mesh.Peer{Name: n.cfg.Name, Online: true}}, true
	}
	kindFilter, name := n.splitTarget(target)

	type snapshot struct {
		ch    mesh.Channel
		self  *mesh.Peer
		peers []mesh.Peer
	}
	var snaps []snapshot
	var cands []candidate
	for _, ch := range n.cfg.Channels {
		if kindFilter != "" && string(ch.Kind()) != kindFilter {
			continue
		}
		st, peers, err := mesh.Snapshot(ctx, ch)
		if err != nil {
			n.logf("获取连接方式 %s 的状态和对端失败: %v", ch.Kind(), err)
			if st.Self == nil {
				continue
			}
		}
		snaps = append(snaps, snapshot{ch: ch, self: st.Self, peers: peers})

		// 本机：名称、主机名、IP 取连接层信息，显示名称就是本节点的设备名。
		self := mesh.Peer{Name: n.cfg.Name, Online: true}
		if st.Self != nil {
			self = *st.Self
		}
		if lvl, ok := matchLevel(self, name, n.cfg.Name); ok {
			cands = append(cands, candidate{resolved{self: true, ch: ch, peer: self}, lvl})
		}
		for _, p := range peers {
			if len(p.Addrs) == 0 {
				continue
			}
			cands = append(cands, n.matchCandidates(ch, p, name)...)
		}
	}

	if len(cands) == 0 {
		// 连接层和缓存都没有匹配：探测在线的同账号对端，再按显示名称匹配。
		for _, s := range snaps {
			for _, p := range n.probeUnknown(ctx, s.ch, s.peers) {
				if displayNameEqual(p.displayName, name) {
					cands = append(cands, candidate{resolved{ch: s.ch, peer: p.peer}, matchDisplayName})
				}
			}
		}
	}
	if len(cands) == 0 {
		return resolved{}, false
	}
	sort.SliceStable(cands, func(i, j int) bool { return betterCandidate(cands[i], cands[j]) })
	return cands[0].resolved, true
}

// matchCandidates 返回对端 p 的匹配候选（0 或 1 个）；显示名称取探测缓存。
func (n *Node) matchCandidates(ch mesh.Channel, p mesh.Peer, name string) []candidate {
	display := ""
	if info, ok := n.probes.peek(probeKey(ch.Kind(), p.ID)); ok {
		display = info.DisplayName
	}
	if lvl, ok := matchLevel(p, name, display); ok {
		return []candidate{{resolved{ch: ch, peer: p}, lvl}}
	}
	return nil
}

// matchLevel 返回 p（DeskBridge 显示名称为 display）匹配 want 的最高级别。
func matchLevel(p mesh.Peer, want, display string) (int, bool) {
	switch {
	case strings.EqualFold(p.Name, want):
		return matchName, true
	case displayNameEqual(display, want):
		return matchDisplayName, true
	case strings.EqualFold(p.HostName, want):
		return matchHostName, true
	}
	for _, a := range p.Addrs {
		if a == want {
			return matchAddr, true
		}
	}
	return 0, false
}

func displayNameEqual(display, want string) bool {
	return display != "" && strings.EqualFold(display, want)
}

// betterCandidate 判断 a 是否应排在 b 前面。
func betterCandidate(a, b candidate) bool {
	if a.level != b.level {
		return a.level < b.level
	}
	if a.self != b.self {
		return a.self
	}
	if a.peer.Mine != b.peer.Mine {
		return a.peer.Mine
	}
	if a.peer.Online != b.peer.Online {
		return a.peer.Online
	}
	if ra, rb := pathRank(a.peer.Path), pathRank(b.peer.Path); ra != rb {
		return ra < rb
	}
	return a.peer.Name < b.peer.Name
}

// probedPeer 是探测出 DeskBridge 显示名称的对端。
type probedPeer struct {
	peer        mesh.Peer
	displayName string
}

// probeUnknown 并发探测在线、同账号、探测缓存里从没有结果的对端（走探测
// 缓存，30 秒内不重复探测），返回探测成功的对端及其显示名称。
func (n *Node) probeUnknown(ctx context.Context, ch mesh.Channel, peers []mesh.Peer) []probedPeer {
	var todo []mesh.Peer
	for _, p := range peers {
		if !p.Mine || !p.Online || len(p.Addrs) == 0 {
			continue
		}
		if _, known := n.probes.peek(probeKey(ch.Kind(), p.ID)); known {
			continue
		}
		todo = append(todo, p)
	}
	out := make([]probedPeer, len(todo))
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for i, p := range todo {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return compactProbed(out)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := n.probes.do(ctx, probeKey(ch.Kind(), p.ID), false, func(pctx context.Context) probeResult {
				return n.probePeer(pctx, ch, p)
			})
			if res.info != nil {
				out[i] = probedPeer{peer: p, displayName: res.info.DisplayName}
			}
		}()
	}
	wg.Wait()
	return compactProbed(out)
}

func compactProbed(in []probedPeer) []probedPeer {
	out := in[:0]
	for _, p := range in {
		if p.displayName != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitTarget 把 "kind:名称" 拆成连接方式和名称；没有连接方式前缀
// （或前缀不是已知的连接方式）时 kind 为空，名称为整个 target。
func (n *Node) splitTarget(target string) (kind, name string) {
	if i := strings.Index(target, ":"); i >= 0 {
		if _, ok := n.findChannel(target[:i]); ok {
			return target[:i], target[i+1:]
		}
	}
	return "", target
}
