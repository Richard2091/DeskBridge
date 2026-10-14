package mcp

// 用 SDK 的内存传输连接服务器；背后是真节点（meshtest.Fake 做连接层），
// 节点上注册假的 proc.exec、fs.* 工具，记录收到的参数。取消的测试用
// 假的本机接口服务，以便确认对端收到了 DELETE。

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/deviceid"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/mesh/meshtest"
	"github.com/Richard2091/DeskBridge/internal/node"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// fakeTools 记录每个工具最近一次收到的参数，并返回预设的结果。
type fakeTools struct {
	mu      sync.Mutex
	args    map[string]map[string]any
	results map[string]any
}

func (f *fakeTools) lastArgs(name string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.args[name]
}

func (f *fakeTools) registry(t *testing.T) *tools.Registry {
	t.Helper()
	f.args = map[string]map[string]any{}
	reg := tools.NewRegistry()
	for name := range f.results {
		name := name
		err := reg.Register(&tools.Tool{
			Spec: protocol.ToolSpec{Name: name, Description: "测试用 " + name, Source: protocol.SourceBuiltin},
			Run: func(_ context.Context, call *tools.Call) (any, error) {
				var a map[string]any
				if len(call.Args) > 0 {
					if err := json.Unmarshal(call.Args, &a); err != nil {
						return nil, err
					}
				}
				f.mu.Lock()
				f.args[name] = a
				f.mu.Unlock()
				if perr, ok := f.results[name].(*protocol.Error); ok {
					return nil, perr
				}
				return f.results[name], nil
			},
		})
		if err != nil {
			t.Fatalf("注册 %s 失败: %v", name, err)
		}
	}
	return reg
}

// startNode 启动带假工具的真节点，返回本机接口 socket 路径。
func startNode(t *testing.T, reg *tools.Registry) string {
	t.Helper()
	dev, err := deviceid.Load(&deviceid.FileStore{Path: filepath.Join(shortTempDir(t), "device.key")})
	if err != nil {
		t.Fatalf("加载设备 ID 失败: %v", err)
	}
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{Kind: mesh.KindTailscale, State: mesh.StateConnected})
	socket := filepath.Join(shortTempDir(t), "node.sock")
	n := node.New(node.Config{
		Channels: []mesh.Channel{fake},
		Device:   dev,
		Tools:    reg,
		Name:     "win-pc",
		Socket:   socket,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("节点没有在 5 秒内退出")
		}
	})
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(socket); err == nil {
			return socket
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("节点没有监听 socket")
	return ""
}

// connect 把 MCP 客户端连到服务器（内存传输），返回客户端会话。
func connect(t *testing.T, c *client.Client) *sdk.ClientSession {
	t.Helper()
	ct, st := sdk.NewInMemoryTransports()
	srv := newServer(&server{c: c, logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = srv.Run(ctx, st) // 退出原因（连接关闭）不影响测试结论
	}()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("MCP 客户端连接失败: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.Close() // 测试结束，关闭错误无意义
		cancel()
		<-srvDone
	})
	return cs
}

// call 调用一个 MCP 工具，返回结果和第一段文本。
func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (*sdk.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("调用 %s 失败: %v", name, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("%s 没有返回内容", name)
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("%s 的内容不是文本: %T", name, res.Content[0])
	}
	return res, tc.Text
}

func newFixture(t *testing.T) (*sdk.ClientSession, *fakeTools) {
	t.Helper()
	ft := &fakeTools{results: map[string]any{
		"proc.exec": map[string]any{"exit_code": 0, "stdout": "你好\n", "stderr": "", "timed_out": false},
		"fs.read":   map[string]any{"path": "/a", "content": "hello", "encoding": "utf-8", "size": 5, "offset": 0, "bytes_read": 5, "eof": true},
		"fs.write":  map[string]any{"path": "/a", "bytes_written": 3, "created": true},
		"fs.list":   map[string]any{"path": "/", "entries": []any{}, "truncated": false},
		"test.fail": protocol.Errorf(protocol.CodePermissionDenied, "没有权限"),
	}}
	socket := startNode(t, ft.registry(t))
	return connect(t, client.New(socket)), ft
}

func TestToolsList(t *testing.T) {
	cs, _ := newFixture(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list 失败: %v", err)
	}
	// 每个工具的必填参数；其余参数必须是可选的。
	want := map[string][]string{
		"list_devices": nil,
		"device_tools": {"device"},
		"call_tool":    {"device", "tool"},
		"exec":         {"device", "command"},
		"read_file":    {"device", "path"},
		"write_file":   {"device", "path", "content"},
		"list_dir":     {"device", "path"},
	}
	if len(res.Tools) != len(want) {
		t.Fatalf("工具数 = %d，期望 %d", len(res.Tools), len(want))
	}
	for _, tool := range res.Tools {
		req, ok := want[tool.Name]
		if !ok {
			t.Errorf("多出工具 %s", tool.Name)
			continue
		}
		if tool.Description == "" {
			t.Errorf("%s 没有描述", tool.Name)
		}
		b, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Type       string                    `json:"type"`
			Required   []string                  `json:"required"`
			Properties map[string]map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatalf("%s 的 schema 解析失败: %v", tool.Name, err)
		}
		if schema.Type != "object" {
			t.Errorf("%s schema.type = %q", tool.Name, schema.Type)
		}
		if strings.Join(schema.Required, ",") != strings.Join(req, ",") {
			t.Errorf("%s required = %v，期望 %v", tool.Name, schema.Required, req)
		}
		for name, p := range schema.Properties {
			if d, _ := p["description"].(string); d == "" {
				t.Errorf("%s 的参数 %s 没有描述", tool.Name, name)
			}
		}
	}
}

func TestExecFormatAndDefaults(t *testing.T) {
	cs, ft := newFixture(t)
	res, text := call(t, cs, "exec", map[string]any{"device": "self", "command": "echo 你好"})
	if res.IsError {
		t.Fatalf("不应是错误: %s", text)
	}
	if want := "退出码：0\n--- 标准输出 ---\n你好\n"; text != want {
		t.Errorf("文本 = %q，期望 %q", text, want)
	}
	args := ft.lastArgs("proc.exec")
	if args["command"] != "echo 你好" || args["max_output_bytes"] != float64(65536) {
		t.Errorf("proc.exec 参数 = %v", args)
	}
	if _, ok := args["shell"]; ok {
		t.Errorf("没指定 shell 时不应传: %v", args)
	}
}

func TestFormatExec(t *testing.T) {
	got := formatExec(execResult{ExitCode: 1, Stdout: "out", Stderr: "err\n", StdoutTruncated: true})
	want := "退出码：1\n--- 标准输出 ---\nout\n--- 标准错误 ---\nerr\n（输出超过上限，已截断）"
	if got != want {
		t.Errorf("得到 %q，期望 %q", got, want)
	}
	got = formatExec(execResult{ExitCode: -1, Stdout: "x\n", TimedOut: true})
	want = "退出码：超时，已终止\n--- 标准输出 ---\nx\n"
	if got != want {
		t.Errorf("得到 %q，期望 %q", got, want)
	}
}

func TestReadFileDefaultsAndHints(t *testing.T) {
	cs, ft := newFixture(t)
	_, text := call(t, cs, "read_file", map[string]any{"device": "self", "path": "/a"})
	if text != "hello" {
		t.Errorf("文本 = %q", text)
	}
	args := ft.lastArgs("fs.read")
	if args["max_bytes"] != float64(262144) || args["encoding"] != "auto" {
		t.Errorf("fs.read 参数 = %v", args)
	}
	if got := formatRead(readResult{Content: "QUJD", Encoding: "base64", Size: 10, BytesRead: 3}); !strings.Contains(got, "二进制文件，内容为 base64") ||
		!strings.Contains(got, "offset=3") {
		t.Errorf("base64/未读完提示缺失: %q", got)
	}
}

func TestWriteAndListDefaults(t *testing.T) {
	cs, ft := newFixture(t)
	res, text := call(t, cs, "write_file", map[string]any{"device": "self", "path": "/a", "content": "中文<>", "append": true})
	if res.IsError || !strings.Contains(text, `"bytes_written": 3`) {
		t.Errorf("write_file 结果 = %q", text)
	}
	if a := ft.lastArgs("fs.write"); a["append"] != true || a["content"] != "中文<>" {
		t.Errorf("fs.write 参数 = %v", a)
	}
	call(t, cs, "list_dir", map[string]any{"device": "self", "path": "/"})
	if a := ft.lastArgs("fs.list"); a["max_entries"] != float64(500) {
		t.Errorf("fs.list 参数 = %v", a)
	}
}

func TestCallToolPrettyJSON(t *testing.T) {
	cs, ft := newFixture(t)
	ft.mu.Lock()
	ft.results["fs.write"] = map[string]any{"note": "a<b>中文"}
	ft.mu.Unlock()
	res, text := call(t, cs, "call_tool", map[string]any{"device": "self", "tool": "fs.write", "args": map[string]any{"path": "/x"}})
	if res.IsError {
		t.Fatalf("不应是错误: %s", text)
	}
	if want := "{\n  \"note\": \"a<b>中文\"\n}"; text != want {
		t.Errorf("文本 = %q，期望 %q", text, want)
	}
}

func TestProtocolErrorBecomesToolError(t *testing.T) {
	cs, _ := newFixture(t)
	res, text := call(t, cs, "call_tool", map[string]any{"device": "self", "tool": "test.fail"})
	if !res.IsError || text != "错误（permission_denied）：没有权限" {
		t.Errorf("IsError=%v 文本=%q", res.IsError, text)
	}
	res, text = call(t, cs, "call_tool", map[string]any{"device": "self", "tool": "no.such"})
	if !res.IsError || !strings.HasPrefix(text, "错误（unknown_tool）：") {
		t.Errorf("IsError=%v 文本=%q", res.IsError, text)
	}
	res, text = call(t, cs, "exec", map[string]any{"device": "no-such-device", "command": "x"})
	if !res.IsError || !strings.HasPrefix(text, "错误（device_not_found）：") {
		t.Errorf("IsError=%v 文本=%q", res.IsError, text)
	}
}

func TestDevicesAndCapability(t *testing.T) {
	cs, _ := newFixture(t)
	res, text := call(t, cs, "list_devices", nil)
	if res.IsError || !strings.Contains(text, `"devices"`) {
		t.Errorf("list_devices: IsError=%v 文本=%q", res.IsError, text)
	}
	res, text = call(t, cs, "device_tools", map[string]any{"device": "self"})
	if res.IsError || !strings.Contains(text, "proc.exec") {
		t.Errorf("device_tools: IsError=%v 文本=%q", res.IsError, text)
	}
}

func TestNodeNotRunning(t *testing.T) {
	c := client.New(filepath.Join(shortTempDir(t), "none.sock"))
	cs := connect(t, c)
	for name, args := range map[string]map[string]any{
		"list_devices": nil,
		"device_tools": {"device": "self"},
		"exec":         {"device": "self", "command": "x"},
	} {
		res, text := call(t, cs, name, args)
		if !res.IsError || text != "本机节点未运行，先运行 deskbridge node" {
			t.Errorf("%s: IsError=%v 文本=%q", name, res.IsError, text)
		}
	}
	// 服务器没有退出：还能列出工具。
	if _, err := cs.ListTools(context.Background(), nil); err != nil {
		t.Errorf("节点未运行后服务器不应退出: %v", err)
	}
}

// TestCancelNotifiesPeer 验证客户端取消调用时，服务器向对端发 DELETE。
func TestCancelNotifiesPeer(t *testing.T) {
	var (
		mu        sync.Mutex
		callID    string
		initiator *protocol.Initiator
		deleted   = make(chan string, 1)
	)
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /local/devices/win-pc/v1/tools/proc.exec", func(w http.ResponseWriter, r *http.Request) {
		var creq protocol.CallRequest
		_ = json.NewDecoder(r.Body).Decode(&creq) // 解析失败时下面的断言会暴露
		mu.Lock()
		callID, initiator = creq.RequestID, creq.Initiator
		mu.Unlock()
		close(started)
		<-r.Context().Done() // 一直挂着，直到客户端断开
	})
	mux.HandleFunc("DELETE /local/devices/win-pc/v1/requests/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleted <- r.PathValue("id")
		w.WriteHeader(http.StatusOK)
	})
	socket := filepath.Join(shortTempDir(t), "fake.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	cs := connect(t, client.New(socket))
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "exec", Arguments: map[string]any{"device": "win-pc", "command": "sleep 100"}})
		errc <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("对端没有收到调用")
	}
	cancel()
	select {
	case id := <-deleted:
		mu.Lock()
		defer mu.Unlock()
		if id == "" || id != callID {
			t.Errorf("取消的请求 ID = %q，调用的是 %q", id, callID)
		}
		if initiator == nil || initiator.Type != "ai" || initiator.Via != "mcp" {
			t.Errorf("发起者 = %+v", initiator)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("对端没有收到取消")
	}
	<-errc
}
