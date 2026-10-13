package client

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// startSocketServer 在临时目录的 unix socket 上起一个 HTTP 服务，
// handler 由测试提供。用 os.MkdirTemp 保证路径足够短（Windows 上
// AF_UNIX 路径不能超过 108 字符）。
func startSocketServer(t *testing.T, h http.Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "node.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("监听 socket 失败: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return socket
}

func TestCallSuccessAndProtocolError(t *testing.T) {
	socket := startSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/local/devices/win-pc/v1/tools/test.echo":
			var creq protocol.CallRequest
			if err := json.NewDecoder(r.Body).Decode(&creq); err != nil {
				t.Errorf("解析请求失败: %v", err)
			}
			if creq.TimeoutMS != 1234 {
				t.Errorf("timeout_ms = %d, 期望 1234", creq.TimeoutMS)
			}
			writeJSON(t, w, protocol.CallResponse{
				OK:         true,
				RequestID:  "r-1",
				Result:     mustJSON(t, map[string]string{"echo": "你好"}),
				DurationMS: 5,
			})
		case r.URL.Path == "/local/devices/win-pc/v1/tools/no.such":
			w.WriteHeader(protocol.HTTPStatus(protocol.CodeUnknownTool))
			writeJSON(t, w, protocol.CallResponse{
				OK:    false,
				Error: protocol.Errorf(protocol.CodeUnknownTool, "没有这个工具：%s", "no.such"),
			})
		default:
			t.Errorf("意外路径 %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	c := New(socket)
	ctx := context.Background()

	out, err := c.Call(ctx, "win-pc", "test.echo", map[string]any{"text": "hi"},
		CallOptions{RequestID: "r-1", TimeoutMS: 1234})
	if err != nil {
		t.Fatalf("Call 失败: %v", err)
	}
	if !out.OK || out.RequestID != "r-1" {
		t.Errorf("响应不对: %+v", out)
	}

	_, err = c.Call(ctx, "win-pc", "no.such", nil, CallOptions{})
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.CodeUnknownTool {
		t.Errorf("Call(no.such) = %v, 期望 *protocol.Error(unknown_tool)", err)
	}
}

func TestErrNotRunning(t *testing.T) {
	dir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	c := New(filepath.Join(dir, "absent.sock"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = c.Call(ctx, "win-pc", "test.echo", nil, CallOptions{})
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("Call = %v, 期望 ErrNotRunning", err)
	}
	if _, err := c.Capability(ctx, "win-pc"); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Capability = %v, 期望 ErrNotRunning", err)
	}
	if err := c.Cancel(ctx, "win-pc", "r-1"); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Cancel = %v, 期望 ErrNotRunning", err)
	}
}

func TestCapabilityAndCancel(t *testing.T) {
	socket := startSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/local/devices/win-pc/v1/capability":
			writeJSON(t, w, protocol.Capability{
				Protocol: protocol.Version,
				DeviceID: "dev-1",
				Name:     "win-pc",
				Tools:    []protocol.ToolSpec{},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/local/devices/win-pc/v1/requests/r-9":
			writeJSON(t, w, map[string]bool{"ok": true})
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	c := New(socket)
	ctx := context.Background()

	capability, err := c.Capability(ctx, "win-pc")
	if err != nil {
		t.Fatalf("Capability 失败: %v", err)
	}
	if capability.Name != "win-pc" || capability.Protocol != protocol.Version {
		t.Errorf("capability 不对: %+v", capability)
	}
	if err := c.Cancel(ctx, "win-pc", "r-9"); err != nil {
		t.Errorf("Cancel 失败: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		t.Errorf("写响应失败: %v", err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
