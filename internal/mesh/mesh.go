// Package mesh 是连接层：登录、对端列表、拨号和监听、身份识别、事件。
//
// 节点协议、设备列表、MCP 和界面只依赖本包定义的 Channel 接口，不直接使用
// tsnet（ADR-0007）。Tailscale（tsnet）是第一种实现；局域网配对（任务 28）和
// WireGuard（任务 19）是以后的实现。各连接方式互相独立，不跨连接方式合并
// （ADR-0010）。
package mesh

import (
	"context"
	"errors"
	"net"
	"time"
)

// Kind 表示一种连接方式。
type Kind string

// 已知的连接方式。
const (
	KindTailscale Kind = "tailscale"
	KindLAN       Kind = "lan"
	KindWireGuard Kind = "wireguard"
	// KindLocal 不是连接方式，表示经本机接口发来的请求（同一用户的本机进程）。
	KindLocal Kind = "local"
)

// State 是连接方式的整体状态。
type State string

// 连接方式的状态。
const (
	StateStarting     State = "starting"     // 正在启动，状态未知
	StateNeedsLogin   State = "needs_login"  // 需要登录，Status.LoginURL 可能给出登录链接
	StateConnected    State = "connected"    // 已加入内网
	StateDisconnected State = "disconnected" // 断开（网络问题、已退出登录等），原因见 Status.Reason
	StateStopped      State = "stopped"      // 未启动或已关闭
)

// Status 是某种连接方式当前的状态。
type Status struct {
	Kind     Kind   `json:"kind"`
	State    State  `json:"state"`
	LoginURL string `json:"login_url,omitempty"` // 需要登录时，用户在浏览器中打开的链接
	Reason   string `json:"reason,omitempty"`    // 断开或出错的原因，给人看
	Account  string `json:"account,omitempty"`   // 登录的账号；没有账号概念的连接方式为空
	Self     *Peer  `json:"self,omitempty"`      // 本机在该连接方式中的信息；未连接时为空
}

// PathKind 是到对端的路径类型，供功能分层判断（ADR-0006）。
type PathKind string

// 路径类型。
const (
	PathUnknown PathKind = "unknown"    // 还没有建立连接或没有测量
	PathLAN     PathKind = "lan_direct" // 局域网直连
	PathDirect  PathKind = "direct"     // 跨网直连（打洞成功）
	PathRelay   PathKind = "relay"      // 经中继，位置见 Peer.Relay
)

// 对端的客户端类型（Peer.Client 的取值）。
const (
	ClientDeskBridge = "deskbridge" // DeskBridge 内置节点（Hostinfo.App 为 deskbridge）
	ClientTSNet      = "tsnet"      // 其他用 tsnet 的程序（Package 为 tsnet，但不是 DeskBridge）
	ClientTailscale  = "tailscale"  // 系统 Tailscale 客户端或其他
)

// Peer 是连接方式中的一个对端，不论它是否运行 DeskBridge。
type Peer struct {
	ID       string   `json:"id"`                 // 连接方式内的稳定 ID
	Name     string   `json:"name"`               // 内网中的短名，可用于寻址
	HostName string   `json:"host_name"`          // 对端报告的系统主机名
	Addrs    []string `json:"addrs"`              // 内网地址（不含端口），第一个用于拨号
	OS       string   `json:"os,omitempty"`       // 对端报告的系统，如 windows、linux
	Owner    string   `json:"owner,omitempty"`    // 主人账号或配对关系
	Mine     bool     `json:"mine"`               // 与本机同一个主人（初版只信任这类对端）
	Online   bool     `json:"online"`             // 连接方式认为对端在线
	Path     PathKind `json:"path"`               // 当前路径
	Relay    string   `json:"relay,omitempty"`    // 中继位置，如 hkg
	Endpoint string   `json:"endpoint,omitempty"` // 直连时对端的实际地址（IP:端口），只用于诊断显示
	// Client 是对端的客户端类型：deskbridge（DeskBridge 内置节点）、tsnet
	//（其他 tsnet 程序）、tailscale（系统 Tailscale 客户端或其他）；读不到时为空。
	// 只有 Tailscale 连接方式填写。
	Client string `json:"client,omitempty"`
	// LatencyMS 是最近一次测量的往返延迟（毫秒），0 表示未测量。
	LatencyMS float64   `json:"latency_ms,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitzero"`
}

// Identity 是连接层查出的请求来源，各连接方式返回同一结构（ADR-0007）。
type Identity struct {
	Kind      Kind   `json:"kind"`
	Owner     string `json:"owner"`                // 账号或配对关系
	OwnerName string `json:"owner_name,omitempty"` // 显示名
	Device    string `json:"device"`               // 对端设备名
	PeerID    string `json:"peer_id"`              // 连接方式内的 ID
	Mine      bool   `json:"mine"`                 // 与本机同一个主人
}

// EventType 是连接层事件的类型。
type EventType string

// 事件类型。
const (
	EventState       EventType = "state"        // 本机状态变化（登录、断开、重连）
	EventPeerOnline  EventType = "peer_online"  // 对端上线或新出现
	EventPeerOffline EventType = "peer_offline" // 对端下线或消失
	EventPeerPath    EventType = "peer_path"    // 对端路径变化（如中继变直连）
)

// Event 是连接层事件。
type Event struct {
	Type   EventType `json:"type"`
	Time   time.Time `json:"time"`
	Status *Status   `json:"status,omitempty"` // EventState 时有值
	Peer   *Peer     `json:"peer,omitempty"`   // 对端事件时有值
}

// Channel 是一种连接方式。实现必须可以并发调用。
type Channel interface {
	// Kind 返回连接方式。
	Kind() Kind

	// Start 启动连接方式并立即返回，不等待登录完成。已有登录状态时自动重连。
	Start(ctx context.Context) error

	// Status 返回当前状态。
	Status(ctx context.Context) (Status, error)

	// Login 发起交互式登录，返回包含登录链接的状态。已经连接时直接返回当前状态。
	Login(ctx context.Context) (Status, error)

	// Logout 退出登录，之后状态为 StateNeedsLogin。
	Logout(ctx context.Context) error

	// Peers 返回所有对端（不含本机）。路径和延迟来自连接层已有的信息，不主动测量。
	Peers(ctx context.Context) ([]Peer, error)

	// Probe 主动测量到对端的路径和延迟，返回更新后的对端信息。addr 是内网地址。
	Probe(ctx context.Context, addr string) (Peer, error)

	// Dial 经内网连接对端，network 为 "tcp"，addr 为 "内网地址:端口"。
	Dial(ctx context.Context, network, addr string) (net.Conn, error)

	// Listen 在本机的内网地址上监听，network 为 "tcp"，addr 如 ":7465"。
	Listen(network, addr string) (net.Listener, error)

	// WhoIs 根据连接的远端地址（net.Conn.RemoteAddr 或 http.Request.RemoteAddr）
	// 查出对方身份。查不到时返回 ErrUnknownPeer。
	WhoIs(ctx context.Context, remoteAddr string) (Identity, error)

	// Subscribe 返回事件通道，ctx 结束时关闭。事件可能合并或丢弃，不保证逐条送达，
	// 订阅方需要时应重新读取 Status 和 Peers。
	Subscribe(ctx context.Context) <-chan Event

	// Close 关闭连接方式，释放资源。登录状态保留，下次 Start 自动恢复。
	Close() error
}

// 连接层的通用错误。
var (
	ErrNotConnected = errors.New("mesh: 连接方式未连接")
	ErrUnknownPeer  = errors.New("mesh: 无法识别对端身份")
	ErrPeerNotFound = errors.New("mesh: 找不到对端")
)
