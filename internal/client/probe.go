package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// ProbeResult 是本机接口 GET /local/probe/{target} 的响应。
type ProbeResult struct {
	Self         bool      `json:"self,omitempty"`  // 目标是本机，没有做测量
	Peer         mesh.Peer `json:"peer"`            // 测量后的对端信息（路径、延迟）
	CapabilityMS float64   `json:"capability_ms"`   // 请求对端 /v1/capability 的耗时，失败时为 0
	DeskBridge   bool      `json:"deskbridge"`      // 对端的 DeskBridge 是否响应
	Error        string    `json:"error,omitempty"` // 测量或请求失败的原因
}

// Probe 测量到目标设备的路径和延迟，并确认对端 DeskBridge 是否响应。
func (c *Client) Probe(ctx context.Context, target string) (ProbeResult, error) {
	var out ProbeResult
	err := c.do(ctx, http.MethodGet, "/local/probe/"+url.PathEscape(target), nil, &out)
	return out, err
}
