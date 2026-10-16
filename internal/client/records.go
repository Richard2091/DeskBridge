package client

// 操作记录的查询、跨设备合并和实时事件订阅。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// RecordQuery 是对一台设备的记录查询条件，零值字段不参与筛选。
type RecordQuery struct {
	Since, Until int64  // started_at 范围（Unix 毫秒）
	OperatorType string // ai / human
	Tool         string // 以 * 结尾为前缀匹配
	Status       string // running / ok / failed / 具体错误码
	Role         string // target / source
	RequestID    string
	Before       int64 // 翻页游标，取上一页的 NextBefore
	Limit        int
}

func (q RecordQuery) values() url.Values {
	v := url.Values{}
	set := func(k, s string) {
		if s != "" {
			v.Set(k, s)
		}
	}
	num := func(k string, n int64) {
		if n > 0 {
			v.Set(k, strconv.FormatInt(n, 10))
		}
	}
	num("since", q.Since)
	num("until", q.Until)
	set("operator_type", q.OperatorType)
	set("tool", q.Tool)
	set("status", q.Status)
	set("role", q.Role)
	set("request_id", q.RequestID)
	num("before", q.Before)
	num("limit", int64(q.Limit))
	return v
}

// Records 查询 target 设备（self 为本机）上的操作记录。
func (c *Client) Records(ctx context.Context, target string, q RecordQuery) (*protocol.RecordsResponse, error) {
	path := "/v1/records"
	if enc := q.values().Encode(); enc != "" {
		path += "?" + enc
	}
	var out protocol.RecordsResponse
	if err := c.do(ctx, http.MethodGet, devicePath(target, path), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Subscribe 订阅 target 设备上一条正在执行的调用的实时事件。返回的通道依次给出
// start、输出片段、end，调用结束、连接断开或 ctx 取消后关闭。调用已经结束
// （或不存在）时返回 not_found 协议错误。
func (c *Client) Subscribe(ctx context.Context, target, requestID string) (<-chan protocol.Event, error) {
	u := "http://deskbridge.local" + devicePath(target, "/v1/requests/"+url.PathEscape(requestID)+"/events")
	conn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: c.hc})
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			var e struct {
				Error *protocol.Error `json:"error"`
			}
			if body, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<10)); rerr == nil && json.Unmarshal(body, &e) == nil && e.Error != nil && e.Error.Code != "" {
				return nil, e.Error
			}
			return nil, protocol.Errorf(protocol.CodeInternal, "订阅失败，节点返回状态码 %d", resp.StatusCode)
		}
		var de *dialError
		if errors.As(err, &de) {
			return nil, fmt.Errorf("%w（%s）", ErrNotRunning, de.err)
		}
		return nil, fmt.Errorf("订阅实时事件失败：%w", err)
	}
	conn.SetReadLimit(1 << 20)
	ch := make(chan protocol.Event, 64)
	go func() {
		defer close(ch)
		defer conn.CloseNow()
		for {
			var ev protocol.Event
			if err := wsjson.Read(ctx, conn, &ev); err != nil {
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
			if ev.Type == protocol.EventEnd {
				return
			}
		}
	}()
	return ch, nil
}

// ActivityQuery 是跨设备操作记录的查询条件。
type ActivityQuery struct {
	Since        int64
	OperatorType string
	Tool         string
	Status       string
	Limit        int // 合并后最多返回多少条，默认 200；每台设备最多取同样多
}

// ActivityItem 是合并后的一条记录。
type ActivityItem struct {
	protocol.Record
	// TargetName 是被控设备的显示名（被控端记录取自设备列表，控制端记录取自记录本身）。
	TargetName string `json:"target_name"`
	// SourceDevice 是发起调用的设备：被控端记录里的调用者设备；
	// 只有控制端记录时是记录所在的设备。
	SourceDevice string `json:"source_device,omitempty"`
}

// ActivityResult 是合并后的结果。Errors 记录查询失败的设备（名称 → 原因），
// 不影响其他设备的结果。
type ActivityResult struct {
	Items  []ActivityItem    `json:"items"`
	Errors map[string]string `json:"errors,omitempty"`
}

// perDeviceTimeout 是查询单台设备记录的时间上限，避免一台不可达的设备拖住整体。
const perDeviceTimeout = 8 * time.Second

// Activity 向本机和所有在线的、运行 DeskBridge 的设备查询操作记录，
// 按时间从新到旧合并，按 request_id 去重（被控端记录优先）。
// 离线设备的记录要等它上线后才能看到。
func (c *Client) Activity(ctx context.Context, q ActivityQuery) (*ActivityResult, error) {
	list, err := c.Devices(ctx, false)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	type job struct {
		target string // 传给 Records 的目标
		name   string // 显示名
		self   bool
	}
	var jobs []job
	seen := map[string]bool{}
	for _, g := range list.Groups {
		for _, d := range g.Devices {
			if d.DeskBridge == nil {
				continue
			}
			// 同一台设备可能出现在多个连接方式里，按 DeskBridge 设备 ID 去重。
			if seen[d.DeskBridge.DeviceID] {
				continue
			}
			if !d.Self && !d.Online {
				continue
			}
			seen[d.DeskBridge.DeviceID] = true
			j := job{name: d.DisplayName(), self: d.Self}
			if d.Self {
				j.target = "self"
			} else {
				j.target = string(d.Kind) + ":" + d.Name
			}
			jobs = append(jobs, j)
		}
	}
	if len(jobs) == 0 {
		// 设备列表里没有本机（节点还没连上任何连接方式）时至少查本机。
		jobs = append(jobs, job{target: "self", name: "本机", self: true})
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	res := &ActivityResult{Items: []ActivityItem{}, Errors: map[string]string{}}
	var all []ActivityItem
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, perDeviceTimeout)
			defer cancel()
			rq := RecordQuery{Since: q.Since, OperatorType: q.OperatorType, Tool: q.Tool, Status: q.Status, Limit: limit}
			if !j.self {
				rq.Role = protocol.RoleTarget // 对方作为控制端的记录会在目标设备上以 target 出现
			}
			resp, err := c.Records(cctx, j.target, rq)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				res.Errors[j.name] = errorText(err)
				return
			}
			for _, r := range resp.Records {
				it := ActivityItem{Record: r}
				if r.Role == protocol.RoleTarget {
					it.TargetName = j.name
					it.SourceDevice = r.CallerDevice
				} else {
					it.TargetName = r.Device
					it.SourceDevice = j.name
				}
				all = append(all, it)
			}
		}()
	}
	wg.Wait()
	if len(res.Errors) == 0 {
		res.Errors = nil
	}
	res.Items = mergeActivity(all, limit)
	return res, nil
}

// mergeActivity 按 request_id 去重（被控端记录优先）、按开始时间从新到旧排序并截到 limit 条。
func mergeActivity(all []ActivityItem, limit int) []ActivityItem {
	byID := make(map[string]int, len(all))
	var out []ActivityItem
	for _, it := range all {
		i, dup := byID[it.RequestID]
		if !dup {
			byID[it.RequestID] = len(out)
			out = append(out, it)
			continue
		}
		if out[i].Role != protocol.RoleTarget && it.Role == protocol.RoleTarget {
			out[i] = it
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].StartedAt > out[b].StartedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []ActivityItem{}
	}
	return out
}

func errorText(err error) string {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return perr.Message
	}
	return err.Error()
}
