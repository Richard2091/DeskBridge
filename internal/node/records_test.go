package node

// 操作记录和实时事件的测试：被控端记录、控制端记录、跨设备合并、
// 输出缓冲和 WebSocket 订阅。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/store"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

func openStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "records.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func getRecords(t *testing.T, url string) protocol.RecordsResponse {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out protocol.RecordsResponse
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &out) != nil {
		t.Fatalf("GET %s = %s %s", url, resp.Status, body)
	}
	return out
}

func TestRecordsTargetSide(t *testing.T) {
	db := openStore(t)
	n := New(Config{Device: testDevice(t), Tools: testTools(t), Name: "win-pc", Records: db})
	srv, _ := fixedAPI(t, n)

	// AI 经 MCP 的调用，参数里有敏感字段。
	postCall(t, srv.URL+"/v1/tools/test.echo",
		`{"request_id":"r-ai","args":{"text":"hi password=hunter2"},"initiator":{"type":"ai","via":"mcp","session":"s9"}}`)
	// 人经命令行的调用，参数不合法（失败）。
	postCall(t, srv.URL+"/v1/tools/test.echo", `{"request_id":"r-human","args":{"bad":1}}`)

	all := getRecords(t, srv.URL+"/v1/records")
	if len(all.Records) != 2 {
		t.Fatalf("应有 2 条记录：%+v", all.Records)
	}
	// 从新到旧：第一条是人的失败调用。
	human, ai := all.Records[0], all.Records[1]
	if human.RequestID != "r-human" || human.OperatorType != "human" || human.Via != "cli" || human.Status != protocol.CodeInvalidArgs {
		t.Errorf("人的记录不对：%+v", human)
	}
	if ai.RequestID != "r-ai" || ai.OperatorType != "ai" || ai.Via != "mcp" || ai.Session != "s9" || ai.Status != "ok" || ai.Role != "target" {
		t.Errorf("AI 的记录不对：%+v", ai)
	}
	if ai.EndedAt == 0 || ai.StartedAt == 0 || ai.CallerOwner == "" {
		t.Errorf("时间或调用者缺失：%+v", ai)
	}
	if strings.Contains(ai.Args, "hunter2") || !strings.Contains(ai.Args, "password=***") {
		t.Errorf("参数摘要未脱敏：%q", ai.Args)
	}

	// 筛选。
	if got := getRecords(t, srv.URL+"/v1/records?operator_type=ai"); len(got.Records) != 1 || got.Records[0].RequestID != "r-ai" {
		t.Errorf("按操作者筛选：%+v", got.Records)
	}
	if got := getRecords(t, srv.URL+"/v1/records?status=failed"); len(got.Records) != 1 || got.Records[0].RequestID != "r-human" {
		t.Errorf("按结果筛选：%+v", got.Records)
	}
	// 翻页。
	p1 := getRecords(t, srv.URL+"/v1/records?limit=1")
	if len(p1.Records) != 1 || p1.NextBefore == 0 {
		t.Fatalf("第一页：%+v", p1)
	}
	p2 := getRecords(t, srv.URL+"/v1/records?limit=1&before="+itoa(p1.NextBefore))
	if len(p2.Records) != 1 || p2.Records[0].RequestID != "r-ai" {
		t.Errorf("第二页：%+v", p2)
	}
	// 非法参数。
	resp, err := http.Get(srv.URL + "/v1/records?limit=abc")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("非法 limit 状态码 = %d", resp.StatusCode)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestRecordsRequireSameOwner(t *testing.T) {
	db := openStore(t)
	n := New(Config{Device: testDevice(t), Tools: testTools(t), Name: "win-pc", Records: db})
	srv, box := fixedAPI(t, n)
	other := mineIdentity("x")
	other.Mine = false
	box.set(other)
	resp, err := http.Get(srv.URL + "/v1/records")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("非同账号查询记录状态码 = %d，期望 403", resp.StatusCode)
	}
}

func TestRecordsNoStoreReturnsEmpty(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	if got := getRecords(t, srv.URL+"/v1/records"); len(got.Records) != 0 {
		t.Errorf("没有数据库时应返回空列表：%+v", got)
	}
}

func TestRecordsBothRolesAndActivity(t *testing.T) {
	db := openStore(t)
	_, socket := startNodeWithRecords(t, "win-pc", testTools(t), db)
	c := client.New(socket)
	ai := protocol.Initiator{Type: protocol.InitiatorAI, Via: protocol.ViaMCP, Session: "sess"}
	// 不指定 request_id：控制端补上，两端记录用同一个 ID。
	resp, err := c.Call(context.Background(), "self", "test.echo", map[string]any{"text": "x"}, client.CallOptions{Initiator: &ai})
	if err != nil {
		t.Fatal(err)
	}
	recs, err := c.Records(context.Background(), "self", client.RecordQuery{RequestID: resp.RequestID})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs.Records) != 2 {
		t.Fatalf("同一次调用应有控制端和被控端各一条：%+v", recs.Records)
	}
	roles := map[string]bool{}
	for _, r := range recs.Records {
		roles[r.Role] = true
		if r.OperatorType != "ai" || r.Status != "ok" || r.Tool != "test.echo" {
			t.Errorf("记录不对：%+v", r)
		}
	}
	if !roles["target"] || !roles["source"] {
		t.Errorf("角色不全：%v", roles)
	}

	// 合并后只剩一条，且是被控端记录。
	act, err := c.Activity(context.Background(), client.ActivityQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(act.Items) != 1 || act.Items[0].Role != "target" || act.Items[0].RequestID != resp.RequestID {
		t.Errorf("合并结果：%+v", act.Items)
	}

	// 失败的调用（未知工具）在控制端也有记录，结果码取自被控端的错误。
	_, err = c.Call(context.Background(), "self", "no.such", nil, client.CallOptions{RequestID: "bad-1"})
	if err == nil {
		t.Fatal("未知工具应失败")
	}
	// 未知工具在 handleCall 里先被拒绝，不进被控端记录；控制端记录应有 unknown_tool。
	failed, _ := c.Records(context.Background(), "self", client.RecordQuery{RequestID: "bad-1"})
	if len(failed.Records) != 1 || failed.Records[0].Role != "source" || failed.Records[0].Status != protocol.CodeUnknownTool {
		t.Errorf("失败调用的控制端记录：%+v", failed.Records)
	}
}

func TestNodeMarksInterruptedOnStart(t *testing.T) {
	db := openStore(t)
	if _, err := db.Begin(store.Record{RequestID: "old", Role: store.RoleTarget, OperatorType: "ai", Tool: "x"}); err != nil {
		t.Fatal(err)
	}
	startNodeWithRecords(t, "win-pc", testTools(t), db)
	got, _ := db.Query(store.Filter{RequestID: "old"})
	if len(got) != 1 || got[0].Status != store.StatusInterrupted {
		t.Errorf("节点启动后上次遗留的 running 记录应标为 interrupted：%+v", got)
	}
}

// streamTool 先推送一段输出，等 gate 关闭后再推送一段 stderr 并结束。
func streamTool(gate <-chan struct{}) *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{Name: "test.stream", Description: "推送输出", Source: protocol.SourceBuiltin},
		Run: func(ctx context.Context, call *tools.Call) (any, error) {
			call.Stream("stdout", "你好\n")
			select {
			case <-gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			call.Stream("stderr", "bye\n")
			return map[string]any{"done": true}, nil
		},
	}
}

func TestSubscribeLiveEvents(t *testing.T) {
	gate := make(chan struct{})
	reg := testTools(t)
	mustRegister(t, reg, streamTool(gate))
	db := openStore(t)
	_, socket := startNodeWithRecords(t, "win-pc", reg, db)
	c := client.New(socket)

	callDone := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "self", "test.stream", nil, client.CallOptions{RequestID: "ev-1"})
		callDone <- err
	}()

	// 调用刚发出时可能还没登记，重试订阅。
	var events <-chan protocol.Event
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waitFor(t, func() bool {
		ch, err := c.Subscribe(ctx, "self", "ev-1")
		if err != nil {
			return false
		}
		events = ch
		return true
	}, "订阅不上正在执行的调用")

	next := func() protocol.Event {
		t.Helper()
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("事件通道提前关闭")
			}
			return ev
		case <-time.After(5 * time.Second):
			t.Fatal("等待事件超时")
		}
		return protocol.Event{}
	}
	if ev := next(); ev.Type != protocol.EventStart || ev.Tool != "test.stream" || ev.RequestID != "ev-1" {
		t.Errorf("第一条应是 start：%+v", ev)
	}
	if ev := next(); ev.Type != protocol.EventOutput || ev.Stream != "stdout" || ev.Data != "你好\n" {
		t.Errorf("第二条应是缓冲中的 stdout：%+v", ev)
	}
	close(gate) // 让工具继续
	if ev := next(); ev.Type != protocol.EventOutput || ev.Stream != "stderr" || ev.Data != "bye\n" {
		t.Errorf("第三条应是实时 stderr：%+v", ev)
	}
	if ev := next(); ev.Type != protocol.EventEnd || ev.Code != "ok" {
		t.Errorf("最后应是 end：%+v", ev)
	}
	if _, open := <-events; open {
		t.Error("end 之后通道应关闭")
	}
	if err := <-callDone; err != nil {
		t.Errorf("调用失败：%v", err)
	}

	// 结束后订阅得到 not_found。
	_, err := c.Subscribe(context.Background(), "self", "ev-1")
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.CodeNotFound {
		t.Errorf("订阅已结束的调用 = %v，期望 not_found", err)
	}
}

func TestSubscribeRequiresSameOwner(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	reg := testTools(t)
	mustRegister(t, reg, streamTool(gate))
	n := New(Config{Device: testDevice(t), Tools: reg, Name: "win-pc"})
	srv, box := fixedAPI(t, n)
	go postCall(t, srv.URL+"/v1/tools/test.stream", `{"request_id":"own-1"}`)
	waitFor(t, func() bool { _, ok := n.inflight.Load("own-1"); return ok }, "调用没有进入执行状态")

	other := mineIdentity("x")
	other.Owner = "someone-else"
	box.set(other)
	_, resp, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/requests/own-1/events", nil)
	if err == nil {
		t.Fatal("其他账号不应能订阅")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("状态码 = %v，期望 403", resp)
	}
}

func TestCallEventsBufferCap(t *testing.T) {
	ev := newCallEvents("r", "t", time.Now())
	chunk := strings.Repeat("a", 64<<10)
	for i := 0; i < 10; i++ { // 640 KiB，超过 256 KiB 的上限
		ev.emit("stdout", chunk)
	}
	start, replay, sub, end := ev.subscribe()
	if end != nil || sub == nil {
		t.Fatal("未结束的调用应返回订阅者")
	}
	total := 0
	for _, r := range replay {
		total += len(r.Data)
	}
	if total > eventBufferBytes {
		t.Errorf("缓冲 %d 字节超过上限 %d", total, eventBufferBytes)
	}
	if start.DroppedBytes != int64(10*len(chunk)-total) || start.DroppedBytes == 0 {
		t.Errorf("DroppedBytes = %d，缓冲 %d", start.DroppedBytes, total)
	}
	ev.finish("ok", 5)
	if _, open := <-sub.ch; open { // 先收到 end
		if _, open2 := <-sub.ch; open2 {
			t.Error("end 之后订阅者通道应关闭")
		}
	}
	// 已结束后再订阅：没有订阅者，直接给出 end。
	_, _, sub2, end2 := ev.subscribe()
	if sub2 != nil || end2 == nil || end2.Code != "ok" {
		t.Errorf("结束后订阅 = %v, %v", sub2, end2)
	}
}

func TestSlowSubscriberDropped(t *testing.T) {
	ev := newCallEvents("r", "t", time.Now())
	_, _, sub, _ := ev.subscribe()
	for i := 0; i < subscriberQueue+10; i++ {
		ev.emit("stdout", "x")
	}
	// 队列塞满后订阅者被断开：通道最终关闭。
	n := 0
	for range sub.ch {
		n++
	}
	if n != subscriberQueue {
		t.Errorf("断开前应已收到 %d 条，实际 %d", subscriberQueue, n)
	}
}

// 防止 wsjson 导入在没有用到时被删：握手后收到的第一条消息是 start。
func TestEventsWebSocketHandshake(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	reg := testTools(t)
	mustRegister(t, reg, streamTool(gate))
	n := New(Config{Device: testDevice(t), Tools: reg, Name: "win-pc"})
	srv, _ := fixedAPI(t, n)
	go postCall(t, srv.URL+"/v1/tools/test.stream", `{"request_id":"ws-1"}`)
	waitFor(t, func() bool { _, ok := n.inflight.Load("ws-1"); return ok }, "调用没有进入执行状态")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/requests/ws-1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var ev protocol.Event
	if err := wsjson.Read(ctx, conn, &ev); err != nil || ev.Type != protocol.EventStart {
		t.Fatalf("第一条消息 = %+v, %v", ev, err)
	}
}
