package node

// 本机接口：unix socket 上的 HTTP 服务，供同一用户的本机进程
//（命令行、MCP、桌面客户端）调用。

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/version"
)

// localHandler 返回本机接口的 HTTP 服务。
func (n *Node) localHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /local/status", n.handleLocalStatus)
	mux.HandleFunc("POST /local/channels/{kind}/login", n.handleChannelWrite(n.loginChannel))
	mux.HandleFunc("POST /local/channels/{kind}/logout", n.handleChannelWrite(n.logoutChannel))
	mux.HandleFunc("GET /local/channels/{kind}/peers", n.handleChannelRead(n.peersChannel))
	mux.HandleFunc("/local/devices/{target}/{rest...}", n.handleForward)
	return mux
}

// findChannel 按 kind 查找连接方式。
func (n *Node) findChannel(kind string) (mesh.Channel, bool) {
	for _, ch := range n.cfg.Channels {
		if string(ch.Kind()) == kind {
			return ch, true
		}
	}
	return nil, false
}

func (n *Node) handleLocalStatus(w http.ResponseWriter, r *http.Request) {
	out := client.LocalStatus{
		DeviceID: n.cfg.Device.ID(),
		Name:     n.cfg.Name,
		Version:  version.Version,
		Channels: make([]mesh.Status, 0, len(n.cfg.Channels)),
	}
	for _, ch := range n.cfg.Channels {
		st, err := ch.Status(r.Context())
		if err != nil {
			st = mesh.Status{Kind: ch.Kind(), State: mesh.StateStarting, Reason: err.Error()}
		}
		if st.Kind == "" {
			st.Kind = ch.Kind()
		}
		out.Channels = append(out.Channels, st)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleChannelRead 包装需要按 kind 找连接方式的 GET 接口。
func (n *Node) handleChannelRead(h func(http.ResponseWriter, *http.Request, mesh.Channel)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ch, ok := n.findChannel(r.PathValue("kind"))
		if !ok {
			writeLocalError(w, http.StatusNotFound, "unknown_channel", "没有这个连接方式")
			return
		}
		h(w, r, ch)
	}
}

// handleChannelWrite 包装需要按 kind 找连接方式的 POST 接口。
func (n *Node) handleChannelWrite(h func(http.ResponseWriter, *http.Request, mesh.Channel)) http.HandlerFunc {
	return n.handleChannelRead(h)
}

func (n *Node) loginChannel(w http.ResponseWriter, r *http.Request, ch mesh.Channel) {
	st, err := ch.Login(r.Context())
	if err != nil {
		writeLocalError(w, http.StatusInternalServerError, "login_failed", err.Error())
		return
	}
	if st.Kind == "" {
		st.Kind = ch.Kind()
	}
	writeJSON(w, http.StatusOK, st)
}

func (n *Node) logoutChannel(w http.ResponseWriter, r *http.Request, ch mesh.Channel) {
	if err := ch.Logout(r.Context()); err != nil {
		writeLocalError(w, http.StatusInternalServerError, "logout_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (n *Node) peersChannel(w http.ResponseWriter, r *http.Request, ch mesh.Channel) {
	peers, err := ch.Peers(r.Context())
	if err != nil {
		writeLocalError(w, http.StatusInternalServerError, "peers_failed", err.Error())
		return
	}
	if peers == nil {
		peers = []mesh.Peer{}
	}
	writeJSON(w, http.StatusOK, peers)
}

// handleForward 把 /local/devices/{target}/{rest} 转发到对端节点的
// http://<内网地址>:<端口>/{rest}。target 为 self 时不经过内网，
// 直接由本节点的节点协议处理器处理，这样单机也能使用全部工具。
func (n *Node) handleForward(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	rest := r.PathValue("rest")
	if target == "self" {
		n.serveSelf(w, r, rest)
		return
	}
	ch, peer, ok := n.resolveTarget(r.Context(), target)
	if !ok {
		writeLocalError(w, http.StatusNotFound, "device_not_found", "找不到设备 "+target)
		return
	}
	addr := net.JoinHostPort(peer.Addrs[0], strconv.Itoa(n.port))
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			out := pr.Out
			out.URL.Scheme = "http"
			out.URL.Host = addr
			out.URL.Path = "/" + rest
			out.URL.RawPath = ""
			out.Host = addr
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, dialAddr string) (net.Conn, error) {
				return ch.Dial(ctx, "tcp", dialAddr)
			},
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			n.logf("转发到 %s（%s）失败: %v", target, addr, err)
			writeLocalError(w, http.StatusBadGateway, "unavailable", "无法连接对端设备 "+target)
		},
	}
	proxy.ServeHTTP(w, r)
}

// serveSelf 用本机的节点协议处理器处理请求（路径已去掉设备前缀）。
// 调用者身份是本机的本地身份，不经过连接层。
func (n *Node) serveSelf(w http.ResponseWriter, r *http.Request, rest string) {
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + rest
	r2.URL.RawPath = ""
	n.selfHandler().ServeHTTP(w, r2)
}

// selfHandler 返回本机直接调用的节点协议服务，身份固定为本机本地身份。
func (n *Node) selfHandler() http.Handler {
	return n.apiHandler(func(context.Context, string) (mesh.Identity, error) {
		return mesh.Identity{
			Kind:   mesh.KindLocal,
			Owner:  "local",
			Device: n.cfg.Name,
			Mine:   true,
		}, nil
	})
}

// resolveTarget 在各连接方式的 对端中查找设备。target 可以是
// "kind:名称"（指定连接方式）或 "名称"；名称匹配对端的 Name、
// HostName（不区分大小写）或内网 IP。
func (n *Node) resolveTarget(ctx context.Context, target string) (mesh.Channel, mesh.Peer, bool) {
	name := target
	kindFilter := ""
	if i := strings.Index(target, ":"); i >= 0 {
		if _, ok := n.findChannel(target[:i]); ok {
			kindFilter, name = target[:i], target[i+1:]
		}
	}
	for _, ch := range n.cfg.Channels {
		if kindFilter != "" && string(ch.Kind()) != kindFilter {
			continue
		}
		peers, err := ch.Peers(ctx)
		if err != nil {
			n.logf("获取连接方式 %s 的对端失败: %v", ch.Kind(), err)
			continue
		}
		for _, p := range peers {
			if len(p.Addrs) == 0 {
				continue
			}
			if matchPeer(p, name) {
				return ch, p, true
			}
		}
	}
	return nil, mesh.Peer{}, false
}

// matchPeer 判断对端是否匹配要找的设备。
func matchPeer(p mesh.Peer, want string) bool {
	if strings.EqualFold(p.Name, want) || strings.EqualFold(p.HostName, want) {
		return true
	}
	for _, a := range p.Addrs {
		if a == want {
			return true
		}
	}
	return false
}
