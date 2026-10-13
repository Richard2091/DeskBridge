// Package client 是本机客户端库：命令行、MCP 服务器和桌面客户端经
// unix socket 调用本机节点的共用逻辑。
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/Richard2091/DeskBridge/internal/mesh"
)

// ErrNotRunning 表示本机节点没有运行（socket 连不上）。
var ErrNotRunning = errors.New("本机节点未运行")

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
				return d.DialContext(ctx, "unix", c.socket)
			},
		},
	}
	return c
}

// APIError 是节点返回的业务错误。
type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// do 执行一次请求并把 JSON 响应解码到 out（out 为 nil 时忽略响应体）。
func (c *Client) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://deskbridge.local"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		// 连接层失败一律视为节点未运行，保留原始错误供诊断。
		return fmt.Errorf("%w（%s）", ErrNotRunning, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error APIError `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&e); err == nil && e.Error.Code != "" {
			return &e.Error
		}
		return &APIError{Code: "http_error", Message: "节点返回状态码 " + strconv.Itoa(resp.StatusCode)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Status 返回节点的整体状态。
func (c *Client) Status(ctx context.Context) (LocalStatus, error) {
	var out LocalStatus
	err := c.do(ctx, http.MethodGet, "/local/status", &out)
	return out, err
}

// Login 让指定连接方式发起登录，返回包含登录链接的状态。
func (c *Client) Login(ctx context.Context, kind string) (mesh.Status, error) {
	var out mesh.Status
	err := c.do(ctx, http.MethodPost, "/local/channels/"+kind+"/login", &out)
	return out, err
}

// Logout 让指定连接方式退出登录。
func (c *Client) Logout(ctx context.Context, kind string) error {
	return c.do(ctx, http.MethodPost, "/local/channels/"+kind+"/logout", nil)
}

// Peers 返回指定连接方式的对端列表。
func (c *Client) Peers(ctx context.Context, kind string) ([]mesh.Peer, error) {
	var out []mesh.Peer
	err := c.do(ctx, http.MethodGet, "/local/channels/"+kind+"/peers", &out)
	return out, err
}

// Get 向对端设备 target 转发 GET 请求，path 为对端上的路径，返回响应体。
func (c *Client) Get(ctx context.Context, target, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://deskbridge.local/local/devices/"+target+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w（%s）", ErrNotRunning, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error APIError `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&e); err == nil && e.Error.Code != "" {
			return nil, &e.Error
		}
		return nil, &APIError{Code: "http_error", Message: "转发请求返回状态码 " + strconv.Itoa(resp.StatusCode)}
	}
	return io.ReadAll(resp.Body)
}
