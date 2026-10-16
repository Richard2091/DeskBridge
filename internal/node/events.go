package node

// 正在执行的调用的实时事件：开始、输出片段、结束。每个调用有一个事件
// 中心，保留最近一段输出（有字节上限，超出丢弃最早的部分），新订阅者
// 先拿到这段再接实时片段（架构文档“操作记录”一节）。订阅通过
// GET /v1/requests/{id}/events 升级为 WebSocket。

import (
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Richard2091/DeskBridge/internal/protocol"

)

const (
	eventBufferBytes = 256 << 10 // 每个调用保留的最近输出
	subscriberQueue  = 256       // 订阅者的待发队列，满了说明它太慢，断开它
)

// callEvents 是一个调用的事件中心，并发安全。
type callEvents struct {
	requestID string
	tool      string
	started   time.Time

	mu      sync.Mutex
	chunks  []protocol.Event // 保留的输出片段，按时间顺序
	size    int
	dropped int64
	subs    map[*subscriber]struct{}
	end     *protocol.Event // 非 nil 表示调用已结束
}

// subscriber 是一个订阅者。ch 被关闭而没收到 end 事件，说明订阅者太慢被断开。
type subscriber struct {
	ch chan protocol.Event
}

func newCallEvents(requestID, tool string, started time.Time) *callEvents {
	return &callEvents{requestID: requestID, tool: tool, started: started, subs: map[*subscriber]struct{}{}}
}

// emit 记录并分发一段输出。
func (c *callEvents) emit(stream, data string) {
	ev := protocol.Event{
		Type: protocol.EventOutput, RequestID: c.requestID,
		Time: time.Now().UnixMilli(), Stream: stream, Data: data,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.end != nil {
		return
	}
	c.chunks = append(c.chunks, ev)
	c.size += len(data)
	// 超出上限时从最早的片段开始丢；至少保留最新一个。
	for c.size > eventBufferBytes && len(c.chunks) > 1 {
		c.size -= len(c.chunks[0].Data)
		c.dropped += int64(len(c.chunks[0].Data))
		c.chunks = c.chunks[1:]
	}
	c.broadcast(ev)
}

// finish 分发结束事件并断开所有订阅者。
func (c *callEvents) finish(code string, durationMS int64) {
	ev := protocol.Event{
		Type: protocol.EventEnd, RequestID: c.requestID,
		Time: time.Now().UnixMilli(), Code: code, DurationMS: durationMS,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.end != nil {
		return
	}
	c.end = &ev
	c.broadcast(ev)
	for s := range c.subs {
		close(s.ch)
		delete(c.subs, s)
	}
}

// broadcast 向所有订阅者发送；队列满的订阅者被断开。调用时持有 c.mu。
func (c *callEvents) broadcast(ev protocol.Event) {
	for s := range c.subs {
		select {
		case s.ch <- ev:
		default:
			close(s.ch)
			delete(c.subs, s)
		}
	}
}

// subscribe 返回开始事件、缓冲中的输出和订阅者。调用已结束时 sub 为 nil，
// end 是结束事件，调用方发完 start 和 replay 后再发 end 即可。
func (c *callEvents) subscribe() (start protocol.Event, replay []protocol.Event, sub *subscriber, end *protocol.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	start = protocol.Event{
		Type: protocol.EventStart, RequestID: c.requestID, Tool: c.tool,
		Time: c.started.UnixMilli(), DroppedBytes: c.dropped,
	}
	replay = append([]protocol.Event(nil), c.chunks...)
	if c.end != nil {
		return start, replay, nil, c.end
	}
	sub = &subscriber{ch: make(chan protocol.Event, subscriberQueue)}
	c.subs[sub] = struct{}{}
	return start, replay, sub, nil
}

func (c *callEvents) unsubscribe(s *subscriber) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.subs[s]; ok {
		delete(c.subs, s)
		close(s.ch)
	}
}

// handleEvents 处理 GET /v1/requests/{id}/events：升级为 WebSocket，
// 推送 start、已缓冲的输出、实时输出，调用结束时推送 end 并关闭。
func (a *api) handleEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := a.identify(w, r)
	if !ok {
		return
	}
	reqID := r.PathValue("id")
	rc, found := a.n.lookupCall(reqID)
	if !found {
		writeProtoError(w, protocol.Errorf(protocol.CodeNotFound, "没有正在执行的请求 %s", reqID))
		return
	}
	if rc.owner != id.Owner {
		writeProtoError(w, sameOwnerRequired("只能订阅同一账号的调用"))
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept 已写出错误响应
	}
	defer conn.CloseNow()
	// 我们只发不收；CloseRead 处理对端的关闭和控制帧，对端断开时取消 ctx。
	ctx := conn.CloseRead(r.Context())

	start, replay, sub, end := rc.events.subscribe()
	if sub != nil {
		defer rc.events.unsubscribe(sub)
	}
	send := func(ev protocol.Event) bool {
		return wsjson.Write(ctx, conn, ev) == nil
	}
	if !send(start) {
		return
	}
	for _, ev := range replay {
		if !send(ev) {
			return
		}
	}
	if sub == nil {
		if end != nil {
			send(*end)
		}
		conn.Close(websocket.StatusNormalClosure, "")
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-sub.ch:
			if !open {
				conn.Close(websocket.StatusTryAgainLater, "订阅者太慢或调用已结束")
				return
			}
			if !send(ev) {
				return
			}
			if ev.Type == protocol.EventEnd {
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}
}
