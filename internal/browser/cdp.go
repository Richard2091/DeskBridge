package browser

// 最小 CDP 客户端：一条 WebSocket 连接，JSON-RPC 风格的消息。
// 只实现节点需要的域，命令和事件都按原始 JSON 处理，不生成整套协议类型。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// cdpCommandTimeout 是单条 CDP 命令自己的超时。调用方的 ctx 可能更长
// （例如 wait 工具），但单条命令不应无限等。
const cdpCommandTimeout = 30 * time.Second

// cdpError 是 CDP 返回的错误。
type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *cdpError) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("CDP 错误 %d：%s（%s）", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("CDP 错误 %d：%s", e.Code, e.Message)
}

// wireMessage 是 CDP 的线上消息格式。
type wireMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
}

// cdpReply 是一次命令的返回。
type cdpReply struct {
	result json.RawMessage
	err    *cdpError
}

// cdpClient 是到浏览器的一条 CDP 连接。
type cdpClient struct {
	conn *websocket.Conn

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan *cdpReply
	// handlers 按方法名分发事件。
	handlers map[string]func(sessionID string, params json.RawMessage)

	closed   chan struct{}
	closeMu  sync.Mutex
	closeErr error
}

// dialCDP 连接浏览器的 WebSocket 端点。
func dialCDP(ctx context.Context, wsURL string) (*cdpClient, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		status := ""
		if resp != nil {
			status = fmt.Sprintf("（HTTP %d）", resp.StatusCode)
		}
		return nil, fmt.Errorf("连接浏览器调试端口失败%s：%w", status, err)
	}
	c := &cdpClient{
		conn:     conn,
		pending:  make(map[int64]chan *cdpReply),
		handlers: make(map[string]func(string, json.RawMessage)),
		closed:   make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// on 注册一个事件处理函数。同一个方法只能注册一次。
func (c *cdpClient) on(method string, fn func(sessionID string, params json.RawMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[method] = fn
}

// call 发一条命令并等待结果，把 result 解码到 out（out 可为 nil）。
func (c *cdpClient) call(ctx context.Context, sessionID, method string, params any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, cdpCommandTimeout)
	defer cancel()

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("编码 CDP 参数失败：%w", err)
		}
		rawParams = b
	}

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *cdpReply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	msg := wireMessage{ID: id, Method: method, Params: rawParams, SessionID: sessionID}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("编码 CDP 请求失败：%w", err)
	}
	c.writeMu.Lock()
	writeErr := c.conn.Write(callCtx, websocket.MessageText, data)
	c.writeMu.Unlock()
	if writeErr != nil {
		return fmt.Errorf("发送 CDP 命令 %s 失败：%w", method, writeErr)
	}

	select {
	case reply := <-ch:
		if reply.err != nil {
			return reply.err
		}
		if out != nil && len(reply.result) > 0 {
			if err := json.Unmarshal(reply.result, out); err != nil {
				return fmt.Errorf("解码 CDP 结果 %s 失败：%w", method, err)
			}
		}
		return nil
	case <-callCtx.Done():
		return fmt.Errorf("等待 CDP 命令 %s 超时：%w", method, callCtx.Err())
	case <-c.closed:
		return fmt.Errorf("浏览器调试连接已关闭：%w", c.err())
	}
}

// readLoop 读取消息并分发。
func (c *cdpClient) readLoop() {
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			c.setClosed(err)
			return
		}
		var msg wireMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch := c.pending[msg.ID]
			c.mu.Unlock()
			if ch != nil {
				ch <- &cdpReply{result: msg.Result, err: msg.Error}
			}
			continue
		}
		if msg.Method == "" {
			continue
		}
		c.mu.Lock()
		fn := c.handlers[msg.Method]
		c.mu.Unlock()
		if fn != nil {
			fn(msg.SessionID, msg.Params)
		}
	}
}

// setClosed 记录连接已关闭，并唤醒所有等待中的命令。
func (c *cdpClient) setClosed(err error) {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	select {
	case <-c.closed:
		return
	default:
	}
	c.closeErr = err
	close(c.closed)
}

// err 返回连接关闭的原因。
func (c *cdpClient) err() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closeErr == nil {
		return errors.New("连接已关闭")
	}
	return c.closeErr
}

// close 关闭连接。
func (c *cdpClient) close() {
	c.setClosed(errors.New("主动关闭"))
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
}
