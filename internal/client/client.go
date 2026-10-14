// Package client 是本机客户端库：命令行、MCP 服务器和桌面客户端经
// unix socket 调用本机节点的共用逻辑。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// ErrNotRunning 表示本机节点没有运行（socket 连不上）。与对端返回的
// 协议错误（*protocol.Error）不同，调用方应区别处理。
var ErrNotRunning = errors.New("本机节点未运行")

// dialError 标记连接本机 socket 阶段的失败，do 用它区分“节点未运行”
// 和请求已发出后的超时、中断。
type dialError struct{ err error }

func (e *dialError) Error() string { return e.err.Error() }
func (e *dialError) Unwrap() error { return e.err }

// timeoutSlack 是客户端 HTTP 超时比请求 timeout_ms 多出的余量，给
// 网络传输和节点组装响应留时间。
const timeoutSlack = 15 * time.Second

// LocalStatus 是本机接口 GET /local/status 的响应，也是本机客户端
// 共用的节点状态描述。
type LocalStatus struct {
	DeviceID string        `json:"device_id"`
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Channels []mesh.Status `json:"channels"`
}

// Client 是本机节点的客户端。
type Client struct {
	socket string
	hc     *http.Client
}

// New 创建连接 socketPath 的客户端。
func New(socketPath string) *Client {
	c := &Client{socket: socketPath}
	c.hc = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				conn, err := d.DialContext(ctx, "unix", c.socket)
				if err != nil {
					return nil, &dialError{err}
				}
				return conn, nil
			},
		},
	}
	return c
}

// do 执行一次请求。body 非 nil 时作为 JSON 请求体发送；out 非 nil 时
// 把 200 响应的 JSON 解码到 out。只有连不上本机 socket 才以 ErrNotRunning
// 返回；ctx 到期或取消以包装了 ctx.Err() 的错误返回，其他连接错误
// 以“连接中断”错误返回；节点返回的错误以 *protocol.Error 返回，调用方可原样展示。
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://deskbridge.local"+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		var de *dialError
		switch {
		case ctx.Err() != nil:
			// 拨号阶段 ctx 到期也算超时，不是节点未运行。
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("等待本机节点响应超时：%w", ctx.Err())
			}
			return fmt.Errorf("等待本机节点响应已取消：%w", ctx.Err())
		case errors.As(err, &de):
			return fmt.Errorf("%w（%s）", ErrNotRunning, de.err)
		default:
			return fmt.Errorf("与本机节点的连接中断：%w", err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error *protocol.Error `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&e); err == nil && e.Error != nil && e.Error.Code != "" {
			return e.Error
		}
		return protocol.Errorf(protocol.CodeInternal, "节点返回状态码 %s", strconv.Itoa(resp.StatusCode))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Status 返回节点的整体状态。
func (c *Client) Status(ctx context.Context) (LocalStatus, error) {
	var out LocalStatus
	err := c.do(ctx, http.MethodGet, "/local/status", nil, &out)
	return out, err
}

// Login 让指定连接方式发起登录，返回包含登录链接的状态。
func (c *Client) Login(ctx context.Context, kind string) (mesh.Status, error) {
	var out mesh.Status
	err := c.do(ctx, http.MethodPost, "/local/channels/"+url.PathEscape(kind)+"/login", nil, &out)
	return out, err
}

// Logout 让指定连接方式退出登录。
func (c *Client) Logout(ctx context.Context, kind string) error {
	return c.do(ctx, http.MethodPost, "/local/channels/"+url.PathEscape(kind)+"/logout", nil, nil)
}

// Peers 返回指定连接方式的对端列表。
func (c *Client) Peers(ctx context.Context, kind string) ([]mesh.Peer, error) {
	var out []mesh.Peer
	err := c.do(ctx, http.MethodGet, "/local/channels/"+url.PathEscape(kind)+"/peers", nil, &out)
	return out, err
}

// devicePath 拼出经本机接口访问对端设备（或 self）的路径。
func devicePath(target, rest string) string {
	return "/local/devices/" + url.PathEscape(target) + rest
}

// Capability 返回对端设备的能力报告。
func (c *Client) Capability(ctx context.Context, target string) (*protocol.Capability, error) {
	var out protocol.Capability
	if err := c.do(ctx, http.MethodGet, devicePath(target, "/v1/capability"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CallOptions 是一次工具调用的可选项。
type CallOptions struct {
	RequestID string              // 为空时由节点生成
	TimeoutMS int                 // 0 用工具默认值
	Initiator *protocol.Initiator // 调用发起者，由调用方声明
}

// Call 调用对端设备上的一个工具。对端返回的协议错误以 *protocol.Error
// 返回；连不上本机节点以 ErrNotRunning 返回。成功时返回完整的 CallResponse。
func (c *Client) Call(ctx context.Context, target, tool string, args any, opts CallOptions) (*protocol.CallResponse, error) {
	var raw json.RawMessage
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("编码调用参数失败: %w", err)
		}
		raw = b
	}
	body, err := json.Marshal(protocol.CallRequest{
		RequestID: opts.RequestID,
		Args:      raw,
		TimeoutMS: opts.TimeoutMS,
		Initiator: opts.Initiator,
	})
	if err != nil {
		return nil, fmt.Errorf("编码调用请求失败: %w", err)
	}
	if opts.TimeoutMS > 0 {
		// HTTP 超时要比请求的超时长一些，否则收不到节点的超时响应。
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.TimeoutMS)*time.Millisecond+timeoutSlack)
		defer cancel()
	}
	var out protocol.CallResponse
	if err := c.do(ctx, http.MethodPost, devicePath(target, "/v1/tools/"+url.PathEscape(tool)), body, &out); err != nil {
		return nil, err
	}
	if !out.OK && out.Error != nil {
		return &out, out.Error
	}
	return &out, nil
}

// Cancel 请求对端取消一条正在执行的调用。
func (c *Client) Cancel(ctx context.Context, target, requestID string) error {
	return c.do(ctx, http.MethodDelete, devicePath(target, "/v1/requests/"+url.PathEscape(requestID)), nil, nil)
}

// Get 向对端设备 target 转发 GET 请求，path 为对端上的路径，返回响应体。
// 新代码请优先用 Capability、Call 等具体方法。
func (c *Client) Get(ctx context.Context, target, path string) ([]byte, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, devicePath(target, path), nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
