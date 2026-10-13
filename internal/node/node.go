// Package node 负责节点：在被控设备上运行，经各连接方式对外提供本阶段
// 的能力描述协议，并通过本机 unix socket 供命令行等本地客户端调用。
package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Richard2091/DeskBridge/internal/deviceid"
	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// 默认节点协议端口。
const defaultPort = 7465

// 等待连接方式就绪的重试间隔。
const retryInterval = 2 * time.Second

// Config 是节点的配置。
type Config struct {
	Channels []mesh.Channel
	Device   *deviceid.Device
	Name     string // 设备名，默认 os.Hostname()
	Port     int    // 节点协议端口，默认 7465
	Socket   string // 本机接口的 unix socket 路径
	Logf     func(format string, args ...any)
}

// Node 是一个被控设备上的节点。
type Node struct {
	cfg  Config
	logf func(format string, args ...any)
	port int

	mu       sync.Mutex
	listener net.Listener
}

// New 创建节点，需要调用 Run 才会开始服务。
func New(cfg Config) *Node {
	if cfg.Name == "" {
		if h, err := os.Hostname(); err == nil && h != "" {
			cfg.Name = h
		} else {
			cfg.Name = "deskbridge"
		}
	}
	port := cfg.Port
	if port == 0 {
		port = defaultPort
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Node{cfg: cfg, logf: logf, port: port}
}

// Run 启动所有连接方式和本机接口并阻塞，直到 ctx 结束。
// socket 已被占用（已有节点在运行）时返回错误。
func (n *Node) Run(ctx context.Context) error {
	ln, err := n.listenSocket()
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.listener = ln
	n.mu.Unlock()
	defer func() {
		ln.Close()
		n.removeSocket()
	}()

	localSrv := &http.Server{Handler: n.localHandler()}
	go func() {
		<-ctx.Done()
		localSrv.Close()
	}()

	for _, ch := range n.cfg.Channels {
		go n.serveChannel(ctx, ch)
	}
	n.logf("节点已启动：设备 %s，本机接口 %s", n.cfg.Device.ID(), n.cfg.Socket)

	if err := localSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && ctx.Err() == nil {
		return fmt.Errorf("本机接口服务失败: %w", err)
	}
	return nil
}

// serveChannel 等一个连接方式连接成功，然后在其上监听并提供内网协议。
// 断开（监听失效）后继续等待重连，重新开始服务。
func (n *Node) serveChannel(ctx context.Context, ch mesh.Channel) {
	for {
		err := ch.Start(ctx)
		if err == nil {
			break
		}
		n.logf("连接方式 %s 启动失败: %v", ch.Kind(), err)
		if !sleepCtx(ctx, retryInterval) {
			return
		}
	}
	defer ch.Close()
	var last mesh.Status
	for {
		if ctx.Err() != nil {
			return
		}
		st, err := ch.Status(ctx)
		if err != nil {
			st = mesh.Status{State: mesh.StateStarting, Reason: err.Error()}
		}
		if st.State != mesh.StateConnected {
			// 只在状态、原因或登录链接变化时输出，避免每次轮询刷屏。
			if st.State != last.State || st.Reason != last.Reason || st.LoginURL != last.LoginURL {
				switch {
				case st.LoginURL != "":
					n.logf("连接方式 %s 需要登录，在浏览器中打开以下链接：%s", ch.Kind(), st.LoginURL)
				case st.Reason != "":
					n.logf("连接方式 %s 尚未连接（%s：%s），等待中", ch.Kind(), st.State, st.Reason)
				default:
					n.logf("连接方式 %s 尚未连接（%s），等待中", ch.Kind(), st.State)
				}
			}
			last = st
			if !sleepCtx(ctx, retryInterval) {
				return
			}
			continue
		}
		ln, err := ch.Listen("tcp", ":"+strconv.Itoa(n.port))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			n.logf("连接方式 %s 监听失败: %v", ch.Kind(), err)
			if !sleepCtx(ctx, retryInterval) {
				return
			}
			continue
		}
		last = st
		n.logf("连接方式 %s 已连接（%s），在内网监听端口 %d", ch.Kind(), st.Account, n.port)
		srv := &http.Server{Handler: n.channelHandler(ch)}
		go func() {
			<-ctx.Done()
			ln.Close()
			srv.Close()
		}()
		if err := srv.Serve(ln); err != nil && ctx.Err() == nil {
			n.logf("连接方式 %s 的服务中断: %v", ch.Kind(), err)
		}
		srv.Close()
		if !sleepCtx(ctx, retryInterval) {
			return
		}
	}
}

// sleepCtx 等待 d 或 ctx 结束；返回 false 表示 ctx 已结束。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// listenSocket 在 Socket 路径上监听 unix socket。文件已存在时：
// 能连上说明已有节点在运行，返回错误；连不上则删除旧文件。
func (n *Node) listenSocket() (net.Listener, error) {
	path := n.cfg.Socket
	if path == "" {
		return nil, errors.New("node: 未配置本机接口 socket 路径")
	}
	if _, err := os.Stat(path); err == nil {
		if c, derr := net.Dial("unix", path); derr == nil {
			c.Close()
			return nil, errors.New("节点已在运行（本机接口 " + path + " 已被占用）")
		}
		if rerr := os.Remove(path); rerr != nil {
			return nil, fmt.Errorf("删除失效的 socket 文件 %s 失败: %w", path, rerr)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("检查 socket 文件 %s 失败: %w", path, err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("监听本机接口 %s 失败: %w", path, err)
	}
	if err := chmodSocket(path); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// removeSocket 在退出时删除 socket 文件。
func (n *Node) removeSocket() {
	if err := os.Remove(n.cfg.Socket); err == nil {
		n.logf("已删除本机接口 %s", n.cfg.Socket)
	}
}
