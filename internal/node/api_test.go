package node

// 节点协议的契约测试：处理器和监听分开，这里直接用 httptest 构造 api，
// 覆盖成功、超时、取消、非法参数、未知工具、权限、重复请求、请求体
// 过大和能力报告。经本机接口和内网转发的端到端测试在本文件末尾。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// identityBox 是测试用的可切换身份：未设置时 WhoIs 失败，设置后返回
// 保存的身份。模拟连接层识别结果的变化。
type identityBox struct {
	mu    sync.Mutex
	id    mesh.Identity
	known bool
}

func (b *identityBox) set(id mesh.Identity) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.id = id
	b.known = true
}

func (b *identityBox) current() (mesh.Identity, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.known {
		return mesh.Identity{}, mesh.ErrUnknownPeer
	}
	return b.id, nil
}

// fixedAPI 构造一个身份可切换的 httptest 服务器，默认身份与阶段 A 的
// 节点测试一致（同一账号），需要时用返回的 box 切换。
func fixedAPI(t *testing.T, n *Node) (*httptest.Server, *identityBox) {
	t.Helper()
	box := &identityBox{}
	box.set(mineIdentity("linux-box"))
	srv := httptest.NewServer(n.apiHandler(func(ctx context.Context, remoteAddr string) (mesh.Identity, error) {
		return box.current()
	}))
	t.Cleanup(srv.Close)
	return srv, box
}

// postCall 向节点协议发一次工具调用，返回响应和解析出的 CallResponse。
func postCall(t *testing.T, url string, body string) (*http.Response, protocol.CallResponse) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	var out protocol.CallResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析响应失败（%s）: %v", resp.Status, err)
	}
	return resp, out
}

// httpDelete 发一次 DELETE 请求（标准库没有 http.Delete）。
func httpDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("构造 DELETE %s 失败: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s 失败: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// wantProtoError 检查响应是期望的协议错误。
func wantProtoError(t *testing.T, resp *http.Response, out protocol.CallResponse, code string) {
	t.Helper()
	if resp.StatusCode != protocol.HTTPStatus(code) {
		t.Errorf("状态码 = %d, 期望 %d（%s）", resp.StatusCode, protocol.HTTPStatus(code), code)
	}
	if out.OK || out.Error == nil || out.Error.Code != code {
		t.Errorf("错误 = %+v, 期望 code=%s", out.Error, code)
	}
}

func newAPINode(t *testing.T) *Node {
	t.Helper()
	return New(Config{
		Device: testDevice(t),
		Tools:  testTools(t),
		Name:   "win-pc",
	})
}

func TestAPICallSuccess(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"args":{"text":"你好"}}`)
	if resp.StatusCode != http.StatusOK || !out.OK {
		t.Fatalf("状态码 = %d, ok = %v（错误 %+v）", resp.StatusCode, out.OK, out.Error)
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatalf("解析 result 失败: %v", err)
	}
	if result.Text != "你好" {
		t.Errorf("result.text = %q, 期望 你好", result.Text)
	}
	if len(out.RequestID) != 32 {
		t.Errorf("自动生成的 request_id 长度 = %d, 期望 32", len(out.RequestID))
	}
}

func TestAPICallExplicitRequestID(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	_, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"request_id":"my-req-1","args":{"text":"x"}}`)
	if out.RequestID != "my-req-1" {
		t.Errorf("request_id = %q, 期望 my-req-1", out.RequestID)
	}
}

func TestAPICallTimeout(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	resp, out := postCall(t, srv.URL+"/v1/tools/test.sleep", `{"args":{"ms":10000},"timeout_ms":50}`)
	wantProtoError(t, resp, out, protocol.CodeTimeout)
	if out.RequestID == "" {
		t.Error("超时响应也应带 request_id")
	}
}

func TestAPICallCancel(t *testing.T) {
	n := newAPINode(t)
	srv, _ := fixedAPI(t, n)

	type callResult struct {
		resp *http.Response
		out  protocol.CallResponse
	}
	done := make(chan callResult, 1)
	go func() {
		resp, out := postCall(t, srv.URL+"/v1/tools/test.sleep", `{"request_id":"req-cancel","args":{"ms":30000}}`)
		done <- callResult{resp, out}
	}()

	// 等请求进入执行状态，然后取消。
	waitFor(t, func() bool {
		_, ok := n.inflight.Load("req-cancel")
		return ok
	}, "请求没有进入执行状态")
	cancelReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/requests/req-cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelResp, err := http.DefaultClient.Do(cancelReq)
	if err != nil {
		t.Fatalf("DELETE 失败: %v", err)
	}
	cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE 状态码 = %d, 期望 200", cancelResp.StatusCode)
	}

	select {
	case r := <-done:
		wantProtoError(t, r.resp, r.out, protocol.CodeCancelled)
	case <-time.After(10 * time.Second):
		t.Fatal("被取消的调用没有返回")
	}

	// 结束后请求记录应被删除，再取消同一个 id 得到 not_found。
	waitFor(t, func() bool {
		_, ok := n.inflight.Load("req-cancel")
		return !ok
	}, "结束后请求记录没有被删除")
	cancelResp2 := httpDelete(t, srv.URL+"/v1/requests/req-cancel")
	if cancelResp2.StatusCode != protocol.HTTPStatus(protocol.CodeNotFound) {
		t.Errorf("取消不存在的请求状态码 = %d, 期望 404", cancelResp2.StatusCode)
	}
}

func TestAPICallInvalidArgs(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	tests := []struct {
		name string
		body string
	}{
		{"未知字段", `{"args":{"nope":"x"}}`},
		{"类型错误", `{"args":{"text":123}}`},
		{"不是JSON", `{oops`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", tt.body)
			wantProtoError(t, resp, out, protocol.CodeInvalidArgs)
		})
	}
	// 空请求体等价于没有参数，不应报错。
	resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", "")
	if resp.StatusCode != http.StatusOK || !out.OK {
		t.Errorf("空请求体应成功：状态码 %d，错误 %+v", resp.StatusCode, out.Error)
	}
}

func TestAPIUnknownTool(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	resp, out := postCall(t, srv.URL+"/v1/tools/no.such", `{}`)
	wantProtoError(t, resp, out, protocol.CodeUnknownTool)
}

func TestAPICallPermissionDenied(t *testing.T) {
	srv, box := fixedAPI(t, newAPINode(t))
	box.set(mesh.Identity{Kind: mesh.KindTailscale, Owner: "mallory@example.com", Device: "evil", Mine: false})
	resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"args":{}}`)
	wantProtoError(t, resp, out, protocol.CodePermissionDenied)
	if out.Error.Details["missing"] != "same_owner" {
		t.Errorf("details = %v, 期望 missing=same_owner", out.Error.Details)
	}

	// WhoIs 失败（无法识别来源）同样拒绝。
	box2 := &identityBox{}
	n2 := newAPINode(t)
	srv2 := httptest.NewServer(n2.apiHandler(func(ctx context.Context, remoteAddr string) (mesh.Identity, error) {
		return box2.current()
	}))
	defer srv2.Close()
	resp2, out2 := postCall(t, srv2.URL+"/v1/tools/test.echo", `{"args":{}}`)
	wantProtoError(t, resp2, out2, protocol.CodePermissionDenied)
}

func TestAPIDuplicateRequest(t *testing.T) {
	n := newAPINode(t)
	srv, _ := fixedAPI(t, n)

	type callResult struct {
		resp *http.Response
		out  protocol.CallResponse
	}
	done := make(chan callResult, 1)
	go func() {
		resp, out := postCall(t, srv.URL+"/v1/tools/test.sleep", `{"request_id":"dup","args":{"ms":30000}}`)
		done <- callResult{resp, out}
	}()
	waitFor(t, func() bool {
		_, ok := n.inflight.Load("dup")
		return ok
	}, "第一个请求没有进入执行状态")

	// 同一个 request_id 的第二个请求应立即被拒绝。
	resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"request_id":"dup","args":{}}`)
	wantProtoError(t, resp, out, protocol.CodeDuplicateRequest)

	// 清理：取消第一个请求。
	httpDelete(t, srv.URL+"/v1/requests/dup")
	<-done
}

func TestAPIPayloadTooLarge(t *testing.T) {
	n := New(Config{
		Device: testDevice(t),
		Tools:  testTools(t),
		Name:   "win-pc",
	})
	// 请求体上限调到很小，避免在测试里构造 16 MiB 的请求。
	a := &api{n: n, whoIs: func(context.Context, string) (mesh.Identity, error) {
		return mineIdentity("linux-box"), nil
	}, maxPayload: 64}
	srv := httptest.NewServer(a.routes())
	defer srv.Close()

	big := `{"args":{"text":"` + strings.Repeat("x", 200) + `"}}`
	resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", big)
	wantProtoError(t, resp, out, protocol.CodePayloadTooLarge)
}

func TestAPICapability(t *testing.T) {
	n := newAPINode(t)
	srv, _ := fixedAPI(t, n)

	resp, err := http.Get(srv.URL + "/v1/capability")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", resp.StatusCode)
	}
	var cap protocol.Capability
	if err := json.NewDecoder(resp.Body).Decode(&cap); err != nil {
		t.Fatal(err)
	}
	if cap.Protocol != protocol.Version || cap.Name != "win-pc" {
		t.Errorf("capability 头部不对: %+v", cap)
	}
	if cap.DeviceID == "" || cap.DeviceID != n.cfg.Device.ID() {
		t.Errorf("device_id = %q, 期望 %q", cap.DeviceID, n.cfg.Device.ID())
	}
	// 公钥应是合法的 base64 且能解出 32 字节的 ed25519 公钥。
	key, err := base64.StdEncoding.DecodeString(cap.PublicKey)
	if err != nil || len(key) != 32 {
		t.Errorf("public_key 解码失败或长度不对: %q（%d 字节，%v）", cap.PublicKey, len(key), err)
	}
	// 注册表里的工具都应出现，按名称排序。
	if len(cap.Tools) != 2 || cap.Tools[0].Name != "test.echo" || cap.Tools[1].Name != "test.sleep" {
		t.Errorf("tools = %+v, 期望 test.echo、test.sleep 按名称排序", cap.Tools)
	}
	for _, ts := range cap.Tools {
		if ts.Source != protocol.SourceBuiltin || ts.Description == "" {
			t.Errorf("工具描述不完整: %+v", ts)
		}
	}
}

func TestAPICancelOtherOwnerDenied(t *testing.T) {
	n := newAPINode(t)
	srv, box := fixedAPI(t, n)
	box.set(mineIdentity("alice-box"))

	type callResult struct {
		resp *http.Response
		out  protocol.CallResponse
	}
	done := make(chan callResult, 1)
	go func() {
		resp, out := postCall(t, srv.URL+"/v1/tools/test.sleep", `{"request_id":"req-owner","args":{"ms":30000}}`)
		done <- callResult{resp, out}
	}()
	waitFor(t, func() bool {
		_, ok := n.inflight.Load("req-owner")
		return ok
	}, "请求没有进入执行状态")

	// 另一个账号的设备不能取消别人的调用。
	box.set(mesh.Identity{Kind: mesh.KindTailscale, Owner: "mallory@example.com", Device: "evil", Mine: true})
	resp := httpDelete(t, srv.URL+"/v1/requests/req-owner")
	if resp.StatusCode != protocol.HTTPStatus(protocol.CodePermissionDenied) {
		t.Fatalf("状态码 = %d, 期望 403", resp.StatusCode)
	}
	var out struct {
		Error protocol.Error `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Details["missing"] != "same_owner" {
		t.Errorf("details = %v, 期望 missing=same_owner", out.Error.Details)
	}

	// 清理：换回发起者的身份取消，让第一个调用结束。
	box.set(mineIdentity("alice-box"))
	httpDelete(t, srv.URL+"/v1/requests/req-owner")
	<-done
}

func TestAPICancelRequiresIdentity(t *testing.T) {
	n := newAPINode(t)
	srv := httptest.NewServer(n.apiHandler(func(context.Context, string) (mesh.Identity, error) {
		return mesh.Identity{}, mesh.ErrUnknownPeer
	}))
	defer srv.Close()

	resp := httpDelete(t, srv.URL+"/v1/requests/whatever")
	if resp.StatusCode != protocol.HTTPStatus(protocol.CodePermissionDenied) {
		t.Errorf("状态码 = %d, 期望 403", resp.StatusCode)
	}
}

// --- 端到端：client → 本机节点 → Fake 内网 → 对端节点 → 工具 ---

func TestEndToEndCallSelf(t *testing.T) {
	// target 为 self 时不经过内网，直接由本节点处理。
	_, socket := startNodeWithTools(t, "win-pc", testTools(t))
	c := client.New(socket)
	initiator := protocol.Initiator{Type: protocol.InitiatorAI, Via: "mcp", Session: "s-1"}
	out, err := c.Call(context.Background(), "self", "test.echo",
		map[string]any{"text": "单机"},
		client.CallOptions{RequestID: "e2e-self", Initiator: &initiator})
	if err != nil {
		t.Fatalf("Call(self) 失败: %v", err)
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Text != "单机" || out.RequestID != "e2e-self" {
		t.Errorf("结果不对: %+v（request_id %q）", result, out.RequestID)
	}
}

func TestEndToEndCallForwarded(t *testing.T) {
	// client → 节点 B 的本机接口 → Fake 内网 → 节点 A → 工具。
	fakeA, _ := startNodeWithTools(t, "node-a", testTools(t))
	fakeA.SetDefaultIdentity(mineIdentity("node-b"))

	socketB := startRelayNode(t, fakeA, []mesh.Peer{{
		ID:    "peer-a",
		Name:  "node-a",
		Addrs: []string{"100.64.0.1"},
		Mine:  true,
	}}, "e2e-b")

	initiator := protocol.Initiator{Type: protocol.InitiatorHuman, Via: "cli"}
	out, err := client.New(socketB).Call(context.Background(), "node-a", "test.echo",
		map[string]any{"text": "转发"},
		client.CallOptions{TimeoutMS: 5000, Initiator: &initiator})
	if err != nil {
		t.Fatalf("转发调用失败: %v", err)
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Text != "转发" {
		t.Errorf("result.text = %q, 期望 转发", result.Text)
	}

	// 转发的调用也能拿到对端的协议错误。
	_, err = client.New(socketB).Call(context.Background(), "node-a", "no.such", nil, client.CallOptions{})
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.CodeUnknownTool {
		t.Errorf("错误 = %v, 期望 unknown_tool", err)
	}
}

func TestEndToEndCancelForwarded(t *testing.T) {
	// 经本机接口转发到对端的调用也能在对端取消。
	fakeA, _ := startNodeWithTools(t, "node-a", testTools(t))
	fakeA.SetDefaultIdentity(mineIdentity("node-b"))
	socketB := startRelayNode(t, fakeA, []mesh.Peer{{
		ID:    "peer-a",
		Name:  "node-a",
		Addrs: []string{"100.64.0.1"},
		Mine:  true,
	}}, "e2e-cancel-b")

	c := client.New(socketB)
	done := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "node-a", "test.sleep",
			map[string]any{"ms": 30000},
			client.CallOptions{RequestID: "e2e-cancel", TimeoutMS: 25000})
		done <- err
	}()
	// 等对端可达，然后轮询取消（请求还没开始执行时对端返回 not_found）。
	waitFor(t, func() bool {
		capability, err := c.Capability(context.Background(), "node-a")
		return err == nil && capability.Name == "node-a"
	}, "对端节点不可达")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Cancel(context.Background(), "node-a", "e2e-cancel"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case err := <-done:
		var perr *protocol.Error
		if !errors.As(err, &perr) || perr.Code != protocol.CodeCancelled {
			t.Errorf("被取消的调用 = %v, 期望 cancelled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("被取消的调用没有返回")
	}
}

func TestEndToEndCapabilityForwarded(t *testing.T) {
	fakeA, _ := startNodeWithTools(t, "node-a", testTools(t))
	fakeA.SetDefaultIdentity(mineIdentity("node-b"))
	socketB := startRelayNode(t, fakeA, []mesh.Peer{{
		ID:    "peer-a",
		Name:  "node-a",
		Addrs: []string{"100.64.0.1"},
		Mine:  true,
	}}, "e2e-cap-b")

	capability, err := client.New(socketB).Capability(context.Background(), "node-a")
	if err != nil {
		t.Fatalf("Capability 失败: %v", err)
	}
	if capability.Name != "node-a" || capability.Protocol != protocol.Version {
		t.Errorf("capability 不对: %+v", capability)
	}
	if len(capability.Tools) != 2 {
		t.Errorf("tools 数量 = %d, 期望 2", len(capability.Tools))
	}
}
