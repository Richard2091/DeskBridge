package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/version"
)

// 默认值：保护 AI 的上下文，输出多了让 AI 分块取。数值写进了工具描述，
// 改这里要同步改 tools.go 里的描述。
const (
	defaultExecOutputBytes = 65536  // exec 的 max_output_bytes
	defaultReadBytes       = 262144 // read_file 的 max_bytes
	defaultListEntries     = 500    // list_dir 的 max_entries
)

const (
	// metaTimeout 是 list_devices、device_tools 这类不带 timeout_ms 的调用的上限。
	metaTimeout = 60 * time.Second
	// cancelTimeout 是通知对端取消调用的时限（此时调用方的 ctx 已取消，要用新的）。
	cancelTimeout = 5 * time.Second
	// notRunningText 是本机节点未运行时返回给 AI 的提示。
	notRunningText = "本机节点未运行，先运行 deskbridge node"
)

// server 是 MCP 服务器的状态：本机节点的客户端和日志函数。
type server struct {
	c    *client.Client
	logf func(format string, args ...any)
}

// stderrLogf 把日志写到标准错误（标准输出被 MCP 协议占用）。
func stderrLogf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "deskbridge mcp: "+format+"\n", args...)
}

// NewServer 创建注册好全部工具的 MCP 服务器。
func NewServer(c *client.Client) *sdk.Server {
	return newServer(&server{c: c, logf: stderrLogf})
}

func newServer(s *server) *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{Name: "deskbridge", Version: version.Version}, nil)
	s.addTools(srv)
	return srv
}

// Serve 在 in/out 上以 MCP 协议提供服务，直到 in 关闭或 ctx 取消。
// 节点没有运行不会让服务退出，只是工具调用返回错误结果。
func Serve(ctx context.Context, c *client.Client, in io.Reader, out io.Writer) error {
	t := &sdk.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{out}}
	return NewServer(c).Run(ctx, t)
}

// nopWriteCloser 让 io.Writer 满足 io.WriteCloser；关闭时不关闭底层 writer。
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// errorResult 把错误变成 isError 的工具结果，让 AI 能读到原因。
func errorResult(msg string) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: msg}}}
}

// textResult 返回成功的文本结果。
func textResult(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

// failure 把调用节点时的错误转成工具结果：协议错误写“错误（code）：message”，
// 节点未运行给固定提示。
func failure(err error) *sdk.CallToolResult {
	var perr *protocol.Error
	switch {
	case errors.As(err, &perr):
		return errorResult(fmt.Sprintf("错误（%s）：%s", perr.Code, perr.Message))
	case errors.Is(err, client.ErrNotRunning):
		return errorResult(notRunningText)
	case errors.Is(err, context.DeadlineExceeded):
		return errorResult("错误：等待本机节点响应超时（设备可能不可达）")
	default:
		return errorResult("错误：" + err.Error())
	}
}

// initiator 返回本次调用的发起者；MCP 会话 ID 取不到时留空。
func initiator(req *sdk.CallToolRequest) *protocol.Initiator {
	in := &protocol.Initiator{Type: protocol.InitiatorAI, Via: "mcp"}
	if req != nil && req.Session != nil {
		in.Session = req.Session.ID()
	}
	return in
}

// callTool 调用 device 上的工具 tool。失败时第二个返回值非 nil，调用方
// 直接把它作为工具结果返回。ctx 被取消（客户端发了取消通知）时，先通知
// 对端取消再返回。
func (s *server) callTool(ctx context.Context, req *sdk.CallToolRequest, device, tool string, args any, timeoutMS int) (*protocol.CallResponse, *sdk.CallToolResult) {
	if device == "" {
		return nil, errorResult("错误（invalid_args）：device 不能为空，先用 list_devices 查看设备")
	}
	if timeoutMS < 0 {
		timeoutMS = 0
	}
	id := protocol.NewRequestID()
	resp, err := s.c.Call(ctx, device, tool, args, client.CallOptions{
		RequestID: id,
		TimeoutMS: timeoutMS,
		Initiator: initiator(req),
	})
	if err != nil {
		if ctx.Err() != nil {
			s.cancelRemote(ctx, device, id)
			return nil, errorResult("错误（cancelled）：调用已被取消")
		}
		return nil, failure(err)
	}
	return resp, nil
}

// cancelRemote 通知对端取消正在执行的调用。ctx 已取消，所以用脱离它的
// 短超时 context。对端已经因连接断开而结束（not_found）不算问题。
func (s *server) cancelRemote(ctx context.Context, device, id string) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancelTimeout)
	defer cancel()
	err := s.c.Cancel(cctx, device, id)
	var perr *protocol.Error
	if err != nil && !(errors.As(err, &perr) && perr.Code == protocol.CodeNotFound) {
		s.logf("通知 %s 取消请求 %s 失败：%v", device, id, err)
	}
}
