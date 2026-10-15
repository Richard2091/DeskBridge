package node

// 本机接口 GET /local/probe/{target}：测量到对端的路径和延迟，并确认对端
// 的 DeskBridge 是否响应，供命令行 ping 和桌面客户端使用。

import (
	"context"
	"net/http"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
)

// 两个测量步骤各自的超时。路径测量超时后仍继续做能力探测。
var (
	pathProbeTimeout  = 10 * time.Second // 连接层 Probe（测量路径和延迟）
	capabilityTimeout = 10 * time.Second // 对端 /v1/capability 请求
)

func (n *Node) handleProbe(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	t, ok := n.resolveTarget(r.Context(), target)
	if !ok {
		writeLocalError(w, http.StatusNotFound, "device_not_found", "找不到设备 "+target)
		return
	}
	if t.self {
		// 本机不经过内网，不测量。
		writeJSON(w, http.StatusOK, client.ProbeResult{Self: true, Peer: t.peer, DeskBridge: true})
		return
	}
	res := client.ProbeResult{Peer: t.peer}
	pctx, cancel := context.WithTimeout(r.Context(), pathProbeTimeout)
	p, err := t.ch.Probe(pctx, t.peer.Addrs[0])
	cancel()
	if err != nil {
		res.Error = "测量路径失败：" + err.Error()
	} else {
		res.Peer = p
	}
	cctx, cancel := context.WithTimeout(r.Context(), capabilityTimeout)
	defer cancel()
	start := time.Now()
	pr := n.probePeer(cctx, t.ch, t.peer)
	elapsed := float64(time.Since(start).Microseconds()) / 1000
	if pr.info == nil {
		if res.Error != "" {
			res.Error += "；"
		}
		res.Error += "DeskBridge 无响应：" + pr.errMsg
	} else {
		res.DeskBridge = true
		res.CapabilityMS = elapsed
	}
	writeJSON(w, http.StatusOK, res)
}
