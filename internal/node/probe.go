package node

// 本机接口 GET /local/probe/{target}：测量到对端的路径和延迟，并确认对端
// 的 DeskBridge 是否响应，供命令行 ping 和桌面客户端使用。

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// probeTimeout 是对端 /v1/capability 请求的超时。
const probeTimeout = 10 * time.Second

func (n *Node) handleProbe(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	if target == "self" {
		// 本机不经过内网，不测量。
		writeJSON(w, http.StatusOK, client.ProbeResult{
			Self:       true,
			Peer:       mesh.Peer{Name: n.cfg.Name, Online: true},
			DeskBridge: true,
		})
		return
	}
	ch, peer, ok := n.resolveTarget(r.Context(), target)
	if !ok {
		writeLocalError(w, http.StatusNotFound, "device_not_found", "找不到设备 "+target)
		return
	}
	res := client.ProbeResult{Peer: peer}
	addr := peer.Addrs[0]
	if p, err := ch.Probe(r.Context(), addr); err != nil {
		res.Error = "测量路径失败：" + err.Error()
	} else {
		res.Peer = p
	}
	ms, err := n.timeCapability(r.Context(), ch, net.JoinHostPort(addr, strconv.Itoa(n.port)))
	if err != nil {
		if res.Error != "" {
			res.Error += "；"
		}
		res.Error += "DeskBridge 无响应：" + err.Error()
	} else {
		res.DeskBridge = true
		res.CapabilityMS = ms
	}
	writeJSON(w, http.StatusOK, res)
}

// timeCapability 经 ch 请求对端 addr（内网地址:端口）的 /v1/capability，
// 返回耗时毫秒数。响应状态不是 200 视为失败。
func (n *Node) timeCapability(ctx context.Context, ch mesh.Channel, addr string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/v1/capability", nil)
	if err != nil {
		return 0, err
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, dialAddr string) (net.Conn, error) {
			return ch.Dial(ctx, "tcp", dialAddr)
		},
	}
	defer tr.CloseIdleConnections()
	start := time.Now()
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return 0, err
	}
	elapsed := float64(time.Since(start).Microseconds()) / 1000
	if resp.StatusCode != http.StatusOK {
		return 0, &statusError{code: resp.StatusCode}
	}
	return elapsed, nil
}

// statusError 表示对端返回了非 200 的状态码。
type statusError struct{ code int }

func (e *statusError) Error() string { return "对端返回状态码 " + strconv.Itoa(e.code) }
