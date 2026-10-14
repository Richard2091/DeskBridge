package client

// 设备列表（GET /local/devices）的数据结构和调用。结构在这里定义，
// 节点端生成、客户端解码共用，避免两处各写一份。

import (
	"context"
	"net/http"
	"time"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// DeskBridgeInfo 是设备上运行的 DeskBridge 的信息，取自对端的能力报告。
type DeskBridgeInfo struct {
	DeviceID    string            `json:"device_id"`
	DisplayName string            `json:"display_name"` // 能力报告里的 name，即“设备名称”
	Version     string            `json:"version"`
	Platform    protocol.Platform `json:"platform"`
	Tools       int               `json:"tools"` // 工具数量
}

// Device 是设备列表里的一台设备（某个连接方式里的一个节点）。
type Device struct {
	// 连接层信息，取自 mesh.Peer。
	Kind      mesh.Kind     `json:"kind"`
	PeerID    string        `json:"peer_id"`
	Name      string        `json:"name"`      // 内网中的短名
	HostName  string        `json:"host_name"` // 系统主机名
	Addrs     []string      `json:"addrs"`     // 内网地址
	OS        string        `json:"os,omitempty"`
	Client    string        `json:"client,omitempty"` // 客户端类型，见 mesh.Peer.Client
	Online    bool          `json:"online"`
	Path      mesh.PathKind `json:"path"`
	Relay     string        `json:"relay,omitempty"`
	LatencyMS float64       `json:"latency_ms,omitempty"` // 0 表示未测量
	LastSeen  time.Time     `json:"last_seen,omitzero"`
	Self      bool          `json:"self,omitempty"` // 本机

	// DeskBridge 在对端运行 DeskBridge 时有值；没运行、探测失败或设备离线时省略，
	// 探测失败的原因在 ProbeError（给人看）。
	DeskBridge *DeskBridgeInfo `json:"deskbridge,omitempty"`
	ProbeError string          `json:"probe_error,omitempty"`

	// Duplicates 是同一连接方式内设备 ID 相同、被合并掉的其他节点名。
	Duplicates []string `json:"duplicates,omitempty"`
}

// DisplayName 返回用于显示和排序的名字：设备名称，没有则用内网短名。
func (d Device) DisplayName() string {
	if d.DeskBridge != nil && d.DeskBridge.DisplayName != "" {
		return d.DeskBridge.DisplayName
	}
	return d.Name
}

// DeviceGroup 是一种连接方式下的设备。不跨连接方式合并（ADR-0010）。
type DeviceGroup struct {
	Kind    mesh.Kind   `json:"kind"`
	Status  mesh.Status `json:"status"` // 不含 Self，本机在 Devices 第一条
	Devices []Device    `json:"devices"`
	// Error 在读取对端列表失败时给出原因；此时 Devices 只含本机（如果有）。
	Error string `json:"error,omitempty"`
}

// DeviceList 是 GET /local/devices 的响应。
type DeviceList struct {
	Groups []DeviceGroup `json:"groups"`
}

// Devices 返回设备列表。refresh 为 true 时忽略节点的探测缓存，重新探测。
func (c *Client) Devices(ctx context.Context, refresh bool) (*DeviceList, error) {
	path := "/local/devices"
	if refresh {
		path += "?refresh=1"
	}
	var out DeviceList
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
