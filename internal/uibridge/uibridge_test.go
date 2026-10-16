package uibridge

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeNode 在 unix socket 上模拟本机节点：设备列表、记录查询、回显和一个 WebSocket。
func fakeNode(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "uib")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "n.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /local/devices", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"groups":[{"kind":"tailscale","devices":[{"kind":"tailscale","name":"win-pc","self":true,"online":true,"deskbridge":{"device_id":"d1","display_name":"我的电脑","version":"x","platform":{"os":"windows","arch":"amd64"},"tools":3}}]}]}`)
	})
	mux.HandleFunc("GET /local/devices/{target}/v1/records", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"records":[{"id":2,"request_id":"b","role":"source","operator_type":"human","device":"srv","tool":"fs.read","status":"ok","started_at":200},{"id":1,"request_id":"a","role":"target","operator_type":"ai","tool":"proc.exec","status":"running","started_at":100,"caller_device":"srv"}]}`)
	})
	mux.HandleFunc("POST /local/devices/{target}/v1/tools/{name}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Auth-Seen", r.Header.Get("Authorization"))
		w.Write(b)
	})
	mux.HandleFunc("GET /local/events", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		wsjson.Write(r.Context(), c, map[string]string{"type": "start"})
		c.Close(websocket.StatusNormalClosure, "")
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

func newBridge(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(Handler(fakeNode(t), "tok"))
	t.Cleanup(srv.Close)
	return srv, "tok"
}

func get(t *testing.T, url, token string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestTokenRequired(t *testing.T) {
	srv, tok := newBridge(t)
	for _, path := range []string{"/api/ping", "/api/activity", "/local/devices"} {
		if resp, _ := get(t, srv.URL+path, ""); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s 无令牌 = %d，期望 401", path, resp.StatusCode)
		}
		if resp, _ := get(t, srv.URL+path, "wrong"); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s 错令牌 = %d，期望 401", path, resp.StatusCode)
		}
	}
	if resp, _ := get(t, srv.URL+"/api/ping", tok); resp.StatusCode != http.StatusOK {
		t.Errorf("ping = %d", resp.StatusCode)
	}
}

func TestProxyToNode(t *testing.T) {
	srv, tok := newBridge(t)
	resp, body := get(t, srv.URL+"/local/devices", tok)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "我的电脑") {
		t.Fatalf("设备列表 = %d %s", resp.StatusCode, body)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/local/devices/self/v1/tools/x", strings.NewReader(`{"a":1}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	b, _ := io.ReadAll(r2.Body)
	if string(b) != `{"a":1}` {
		t.Errorf("POST 请求体没有原样转发：%s", b)
	}
	if r2.Header.Get("X-Auth-Seen") != "" {
		t.Error("令牌不应转给节点")
	}
}

func TestActivityMerged(t *testing.T) {
	srv, tok := newBridge(t)
	resp, body := get(t, srv.URL+"/api/activity?limit=10", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activity = %d %s", resp.StatusCode, body)
	}
	var out struct {
		Items []struct {
			RequestID  string `json:"request_id"`
			TargetName string `json:"target_name"`
			TargetRef  string `json:"target_ref"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.Items[0].RequestID != "b" || out.Items[1].RequestID != "a" {
		t.Fatalf("应按时间从新到旧合并 2 条：%s", body)
	}
	if out.Items[1].TargetName != "我的电脑" || out.Items[1].TargetRef != "self" {
		t.Errorf("被控端记录应带设备名和地址：%+v", out.Items[1])
	}
	if resp, _ := get(t, srv.URL+"/api/activity?limit=x", tok); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("非法 limit = %d", resp.StatusCode)
	}
}

func TestWebSocketThroughProxy(t *testing.T) {
	srv, tok := newBridge(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/local/events", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + tok}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var m map[string]string
	if err := wsjson.Read(ctx, conn, &m); err != nil || m["type"] != "start" {
		t.Fatalf("经桥接读 WebSocket = %v, %v", m, err)
	}
	// 无令牌的握手被拒绝。
	if _, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/local/events", nil); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("无令牌的 WebSocket 应得 401：%v %v", resp, err)
	}
}
