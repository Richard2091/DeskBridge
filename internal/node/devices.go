package node

// 设备列表：GET /local/devices。按连接方式分组，只列同账号的设备，
// 并探测在线设备上有没有运行 DeskBridge。探测结果缓存 30 秒。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// 设备探测参数。测试会临时改小 probeTimeout，其余为固定值。
var (
	probeTimeout  = 3 * time.Second  // 单个对端的探测超时
	probeCacheTTL = 30 * time.Second // 探测结果缓存时间
)

const (
	probeConcurrency = 8               // 同时探测的对端数
	devicesTimeout   = 4 * time.Second // 整个设备列表请求的时间上限
	maxCapabilityLen = 1 << 20         // 能力报告的大小上限
)

// probeResult 是对一个对端的探测结果：info 非空表示运行着 DeskBridge，
// 否则 errMsg 说明原因（给人看）。
type probeResult struct {
	info   *client.DeskBridgeInfo
	errMsg string
}

type probeEntry struct {
	res     probeResult
	expires time.Time
}

// probeFlight 是正在进行的一次探测，同一个对端的并发请求共用它。
type probeFlight struct {
	done chan struct{}
	res  probeResult
}

// probeCache 缓存探测结果并合并并发探测，零值可用。
type probeCache struct {
	mu      sync.Mutex
	entries map[string]probeEntry
	flights map[string]*probeFlight
	// lastInfo 记住每个对端最近一次探测成功的信息，不随缓存过期而清除，
	// 供目标解析按 DeskBridge 显示名称匹配。
	lastInfo map[string]*client.DeskBridgeInfo
}

// probeKey 是探测缓存里一个对端的键。
func probeKey(kind mesh.Kind, peerID string) string {
	return string(kind) + "\x00" + peerID
}

// peek 返回对端最近一次探测成功的 DeskBridge 信息（即使缓存已过期）。
func (c *probeCache) peek(key string) (*client.DeskBridgeInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, ok := c.lastInfo[key]
	return info, ok
}

// do 返回 key 的探测结果。缓存有效且 refresh 为 false 时直接返回；同一个
// key 已有探测在进行时等它的结果；否则运行 run。run 的 context 与请求的
// 取消无关（只受 probeTimeout 限制），这样一个请求取消不会让别的请求
// 拿到“被取消”的结果。等待期间 ctx 结束则返回超时结果（不写入缓存）。
func (c *probeCache) do(ctx context.Context, key string, refresh bool, run func(context.Context) probeResult) probeResult {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]probeEntry)
		c.flights = make(map[string]*probeFlight)
		c.lastInfo = make(map[string]*client.DeskBridgeInfo)
	}
	if e, ok := c.entries[key]; ok && !refresh && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.res
	}
	fl, running := c.flights[key]
	if !running {
		fl = &probeFlight{done: make(chan struct{})}
		c.flights[key] = fl
		go c.run(ctx, key, fl, run)
	}
	c.mu.Unlock()
	select {
	case <-fl.done:
		return fl.res
	case <-ctx.Done():
		return probeResult{errMsg: "探测超时"}
	}
}

// run 执行探测，写入缓存并通知等待者。
func (c *probeCache) run(ctx context.Context, key string, fl *probeFlight, run func(context.Context) probeResult) {
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeTimeout)
	defer cancel()
	res := run(pctx)
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.entries { // 顺便清掉过期项
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = probeEntry{res: res, expires: now.Add(probeCacheTTL)}
	if res.info != nil {
		c.lastInfo[key] = res.info
	}
	delete(c.flights, key)
	c.mu.Unlock()
	fl.res = res
	close(fl.done)
}

// handleDevices 处理 GET /local/devices。
func (n *Node) handleDevices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), devicesTimeout)
	defer cancel()
	refresh := r.URL.Query().Get("refresh") == "1"

	out := client.DeviceList{Groups: make([]client.DeviceGroup, len(n.cfg.Channels))}
	var wg sync.WaitGroup
	for i, ch := range n.cfg.Channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out.Groups[i] = n.deviceGroup(ctx, ch, refresh)
		}()
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, out)
}

// deviceGroup 生成一种连接方式的设备组。连接方式未连接或读不到对端时
// 不失败，只返回状态和（可能为空的）设备列表。
func (n *Node) deviceGroup(ctx context.Context, ch mesh.Channel, refresh bool) client.DeviceGroup {
	st, err := ch.Status(ctx)
	if err != nil {
		st = mesh.Status{Kind: ch.Kind(), State: mesh.StateStarting, Reason: err.Error()}
	}
	if st.Kind == "" {
		st.Kind = ch.Kind()
	}
	self := st.Self
	st.Self = nil // 本机放在设备列表第一条
	group := client.DeviceGroup{Kind: ch.Kind(), Status: st, Devices: []client.Device{}}
	if st.State != mesh.StateConnected {
		return group
	}

	var devs []client.Device
	if self != nil {
		// 本机不经网络探测，DeskBridge 信息直接取本节点自己的。
		d := deviceFromPeer(ch.Kind(), *self)
		d.Self = true
		d.Online = true
		d.DeskBridge = infoFromCapability(n.capability())
		devs = append(devs, d)
	}
	peers, err := ch.Peers(ctx)
	if err != nil {
		n.logf("获取连接方式 %s 的对端失败: %v", ch.Kind(), err)
		group.Error = "获取对端列表失败: " + err.Error()
	}
	devs = append(devs, n.probePeers(ctx, ch, peers, refresh)...)
	group.Devices = sortDevices(mergeDuplicates(devs))
	return group
}

// probePeers 把同账号的对端转成设备，并对在线的并发探测（最多
// probeConcurrency 个）。离线设备不探测。
func (n *Node) probePeers(ctx context.Context, ch mesh.Channel, peers []mesh.Peer, refresh bool) []client.Device {
	var mine []mesh.Peer
	for _, p := range peers {
		if p.Mine {
			mine = append(mine, p)
		}
	}
	devs := make([]client.Device, len(mine))
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for i, p := range mine {
		devs[i] = deviceFromPeer(ch.Kind(), p)
		if !p.Online {
			continue
		}
		if len(p.Addrs) == 0 {
			devs[i].ProbeError = "没有内网地址"
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			devs[i].ProbeError = "探测超时"
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			key := probeKey(ch.Kind(), p.ID)
			res := n.probes.do(ctx, key, refresh, func(pctx context.Context) probeResult {
				return n.probePeer(pctx, ch, p)
			})
			devs[i].DeskBridge = res.info
			devs[i].ProbeError = res.errMsg
		}()
	}
	wg.Wait()
	return devs
}

// probePeer 经连接方式向对端请求能力报告，使用节点共享的传输。拨号失败
// 的原因通过 context 里的记录器取回，用来分类错误。
func (n *Node) probePeer(ctx context.Context, ch mesh.Channel, p mesh.Peer) probeResult {
	addr := net.JoinHostPort(p.Addrs[0], strconv.Itoa(n.port))
	ctx, rec := withDialRecorder(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/v1/capability", nil)
	if err != nil {
		return probeResult{errMsg: "探测失败"}
	}
	resp, err := (&http.Client{Transport: n.transports.get(ch)}).Do(req)
	if err != nil {
		n.logf("探测 %s（%s）失败: %v", p.Name, addr, err)
		switch de := rec.get(); {
		case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded):
			return probeResult{errMsg: "连接超时"}
		case de != nil && errors.Is(de, mesh.ErrNotConnected):
			return probeResult{errMsg: "连接方式未连接"}
		case de != nil:
			return probeResult{errMsg: "未运行 DeskBridge"}
		default:
			return probeResult{errMsg: "探测失败"}
		}
	}
	defer drainClose(resp.Body)
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return probeResult{errMsg: "对端拒绝访问"}
	case resp.StatusCode != http.StatusOK:
		return probeResult{errMsg: "对端返回状态码 " + strconv.Itoa(resp.StatusCode)}
	}
	var report protocol.Capability
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCapabilityLen)).Decode(&report); err != nil || report.DeviceID == "" {
		return probeResult{errMsg: "对端响应无效"}
	}
	return probeResult{info: infoFromCapability(report)}
}

// infoFromCapability 把能力报告转成设备列表里的 DeskBridge 信息。
func infoFromCapability(c protocol.Capability) *client.DeskBridgeInfo {
	return &client.DeskBridgeInfo{
		DeviceID:    c.DeviceID,
		DisplayName: c.Name,
		Version:     c.Version,
		Platform:    c.Platform,
		Tools:       len(c.Tools),
	}
}

// deviceFromPeer 把连接层对端转成设备（连接层部分）。
func deviceFromPeer(kind mesh.Kind, p mesh.Peer) client.Device {
	return client.Device{
		Kind:      kind,
		PeerID:    p.ID,
		Name:      p.Name,
		HostName:  p.HostName,
		Addrs:     p.Addrs,
		OS:        p.OS,
		Client:    p.Client,
		Online:    p.Online,
		Path:      p.Path,
		Relay:     p.Relay,
		LatencyMS: p.LatencyMS,
		LastSeen:  p.LastSeen,
	}
}

// pathRank 数值越小路径越好。
func pathRank(p mesh.PathKind) int {
	switch p {
	case mesh.PathLAN:
		return 0
	case mesh.PathDirect:
		return 1
	case mesh.PathRelay:
		return 2
	default:
		return 3
	}
}

// betterDevice 判断 a 是否比 b 更适合作为同一设备 ID 的保留项：
// 本机优先，其次在线、路径好，最后按内网短名字典序。
func betterDevice(a, b client.Device) bool {
	if a.Self != b.Self {
		return a.Self
	}
	if a.Online != b.Online {
		return a.Online
	}
	if ra, rb := pathRank(a.Path), pathRank(b.Path); ra != rb {
		return ra < rb
	}
	return a.Name < b.Name
}

// mergeDuplicates 把同一连接方式内 DeskBridge 设备 ID 相同的节点合并成
// 一条（保留 betterDevice 认为最好的），被去掉的节点名放进 Duplicates。
// 没有 DeskBridge 信息的设备不参与合并。
func mergeDuplicates(devs []client.Device) []client.Device {
	out := make([]client.Device, 0, len(devs))
	best := make(map[string]int) // 设备 ID → out 中保留项的下标
	for _, d := range devs {
		if d.DeskBridge == nil {
			out = append(out, d)
			continue
		}
		id := d.DeskBridge.DeviceID
		i, seen := best[id]
		if !seen {
			best[id] = len(out)
			out = append(out, d)
			continue
		}
		if betterDevice(d, out[i]) {
			d.Duplicates = append(append([]string(nil), out[i].Duplicates...), out[i].Name)
			out[i] = d
		} else {
			out[i].Duplicates = append(out[i].Duplicates, d.Name)
		}
	}
	for i := range out {
		sort.Strings(out[i].Duplicates)
	}
	return out
}

// sortDevices 排序：本机第一，然后在线的在前，再按显示名称（设备名称，
// 没有就用内网短名，不区分大小写）、内网短名排序。
func sortDevices(devs []client.Device) []client.Device {
	sort.SliceStable(devs, func(i, j int) bool {
		a, b := devs[i], devs[j]
		if a.Self != b.Self {
			return a.Self
		}
		if a.Online != b.Online {
			return a.Online
		}
		if an, bn := strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName()); an != bn {
			return an < bn
		}
		return a.Name < b.Name
	})
	return devs
}

// drainClose 读完（有上限）并关闭响应体，让连接能回到共享传输的空闲池复用。
func drainClose(body io.ReadCloser) {
	io.Copy(io.Discard, io.LimitReader(body, 64<<10))
	body.Close()
}
