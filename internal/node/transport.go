package node

// 经连接方式拨号的共享 HTTP 传输：转发、设备探测和 ping 共用，
// 每种连接方式一个，长期复用，空闲连接超时后自动关闭。

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// 共享传输和服务端的超时。
const (
	transportIdleTimeout = 90 * time.Second // 传输中空闲连接的保留时间
	serverIdleTimeout    = 2 * time.Minute  // HTTP 服务对空闲连接的保留时间
	serverHeaderTimeout  = 30 * time.Second // HTTP 服务读请求头的时间上限
)

// transports 按连接方式惰性创建并缓存 *http.Transport，零值可用。
type transports struct {
	mu sync.Mutex
	m  map[mesh.Kind]*http.Transport
}

// get 返回 ch 的共享传输，第一次调用时创建。
func (t *transports) get(ch mesh.Channel) *http.Transport {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tr, ok := t.m[ch.Kind()]; ok {
		return tr
	}
	if t.m == nil {
		t.m = make(map[mesh.Kind]*http.Transport)
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			c, err := ch.Dial(ctx, "tcp", addr)
			if err != nil {
				if rec, ok := ctx.Value(dialRecorderKey{}).(*dialRecorder); ok {
					rec.set(err)
				}
			}
			return c, err
		},
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     transportIdleTimeout,
	}
	t.m[ch.Kind()] = tr
	return tr
}

// closeIdle 关闭所有传输的空闲连接。
func (t *transports) closeIdle() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tr := range t.m {
		tr.CloseIdleConnections()
	}
}

// dialRecorder 记录一次请求过程中拨号失败的错误，用来区分“连不上”
// 和“连上后出错”。共享传输的 DialContext 从请求的 context 里取它。
type dialRecorder struct {
	mu  sync.Mutex
	err error
}

type dialRecorderKey struct{}

func (r *dialRecorder) set(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
}

func (r *dialRecorder) get() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// withDialRecorder 返回带拨号错误记录器的 context。
func withDialRecorder(ctx context.Context) (context.Context, *dialRecorder) {
	rec := &dialRecorder{}
	return context.WithValue(ctx, dialRecorderKey{}, rec), rec
}
