package browser

// 收集控制台消息和网络请求，供 browser.console、browser.network 查看。
// 都只保留最近的条目，避免长时间运行占满内存。

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// ConsoleEntry 是一条控制台消息。
type ConsoleEntry struct {
	Level string `json:"level"`          // log、info、warning、error、debug
	Text  string `json:"text"`           // 消息文本
	URL   string `json:"url,omitempty"`  // 来源页面
	Line  int    `json:"line,omitempty"` // 行号
	Time  string `json:"time"`           // RFC 3339
}

// consoleBuffer 是控制台消息的环形缓冲。
type consoleBuffer struct {
	mu      sync.Mutex
	entries []ConsoleEntry
	max     int
}

// newConsoleBuffer 创建缓冲。
func newConsoleBuffer(max int) *consoleBuffer {
	if max <= 0 {
		max = 1000
	}
	return &consoleBuffer{max: max}
}

// add 追加一条消息。
func (b *consoleBuffer) add(e ConsoleEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, e)
	if len(b.entries) > b.max {
		b.entries = b.entries[len(b.entries)-b.max:]
	}
}

// list 返回消息；level 为空时不按级别过滤，clear 为 true 时清空缓冲。
func (b *consoleBuffer) list(level string, maxEntries int, clear bool) []ConsoleEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ConsoleEntry, 0, len(b.entries))
	for _, e := range b.entries {
		if level != "" && severity(e.Level) < severity(level) {
			continue
		}
		out = append(out, e)
	}
	if maxEntries > 0 && len(out) > maxEntries {
		out = out[len(out)-maxEntries:]
	}
	if clear {
		b.entries = nil
	}
	return out
}

// severity 把级别映射为数字，便于「至少 warning」这样的过滤。
func severity(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return 1
	case "log":
		return 2
	case "info":
		return 3
	case "warning", "warn":
		return 4
	case "error":
		return 5
	default:
		return 2
	}
}

// NetEntry 是一条网络请求。
type NetEntry struct {
	ID         string `json:"id,omitempty"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	Type       string `json:"type,omitempty"`
	Status     int    `json:"status,omitempty"`
	MIME       string `json:"mime_type,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Failed     bool   `json:"failed,omitempty"`
	Error      string `json:"error,omitempty"`
}

// networkBuffer 是网络请求的环形缓冲。
type networkBuffer struct {
	mu      sync.Mutex
	order   []string
	entries map[string]*NetEntry
	started map[string]float64
	max     int
	last    time.Time
}

// newNetworkBuffer 创建缓冲。
func newNetworkBuffer(max int) *networkBuffer {
	if max <= 0 {
		max = 1000
	}
	return &networkBuffer{
		entries: make(map[string]*NetEntry),
		started: make(map[string]float64),
		max:     max,
		last:    time.Now(),
	}
}

// requestWillBeSent 处理 Network.requestWillBeSent。
func (b *networkBuffer) requestWillBeSent(params json.RawMessage) {
	var ev struct {
		RequestID string  `json:"requestId"`
		Type      string  `json:"type"`
		Timestamp float64 `json:"timestamp"`
		Request   struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"request"`
	}
	if err := json.Unmarshal(params, &ev); err != nil || ev.RequestID == "" {
		return
	}
	e := &NetEntry{ID: ev.RequestID, Method: ev.Request.Method, URL: ev.Request.URL, Type: ev.Type}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last = time.Now()
	if _, ok := b.entries[ev.RequestID]; !ok {
		b.order = append(b.order, ev.RequestID)
	}
	b.entries[ev.RequestID] = e
	b.started[ev.RequestID] = ev.Timestamp
	b.evictLocked()
}

// responseReceived 处理 Network.responseReceived。
func (b *networkBuffer) responseReceived(params json.RawMessage) {
	var ev struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Response  struct {
			Status  int    `json:"status"`
			MIMEType string `json:"mimeType"`
		} `json:"response"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last = time.Now()
	if e := b.entries[ev.RequestID]; e != nil {
		e.Status = ev.Response.Status
		e.MIME = ev.Response.MIMEType
		if e.Type == "" {
			e.Type = ev.Type
		}
	}
}

// loadingFinished 处理 Network.loadingFinished。
func (b *networkBuffer) loadingFinished(params json.RawMessage) {
	var ev struct {
		RequestID         string  `json:"requestId"`
		Timestamp         float64 `json:"timestamp"`
		EncodedDataLength float64 `json:"encodedDataLength"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last = time.Now()
	e := b.entries[ev.RequestID]
	if e == nil {
		return
	}
	e.SizeBytes = int64(ev.EncodedDataLength)
	if start, ok := b.started[ev.RequestID]; ok && ev.Timestamp > start {
		e.DurationMS = int64((ev.Timestamp - start) * 1000)
	}
	delete(b.started, ev.RequestID)
}

// loadingFailed 处理 Network.loadingFailed。
func (b *networkBuffer) loadingFailed(params json.RawMessage) {
	var ev struct {
		RequestID string  `json:"requestId"`
		Timestamp float64 `json:"timestamp"`
		ErrorText string  `json:"errorText"`
		Canceled  bool    `json:"canceled"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last = time.Now()
	e := b.entries[ev.RequestID]
	if e == nil {
		return
	}
	e.Failed = true
	if ev.ErrorText != "" {
		e.Error = ev.ErrorText
	} else if ev.Canceled {
		e.Error = "已取消"
	}
	if start, ok := b.started[ev.RequestID]; ok && ev.Timestamp > start {
		e.DurationMS = int64((ev.Timestamp - start) * 1000)
	}
	delete(b.started, ev.RequestID)
}

// evictLocked 删除超出上限的旧条目。
func (b *networkBuffer) evictLocked() {
	for len(b.order) > b.max {
		id := b.order[0]
		b.order = b.order[1:]
		delete(b.entries, id)
		delete(b.started, id)
	}
}

// idleFor 判断距离最近一次网络活动是否已超过 d。
func (b *networkBuffer) idleFor(d time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Since(b.last) >= d
}

// list 返回请求列表；filter 可取 all、xhr、document、failed。
func (b *networkBuffer) list(filter string, maxEntries int) []NetEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]NetEntry, 0, len(b.order))
	for _, id := range b.order {
		e := b.entries[id]
		if e == nil {
			continue
		}
		switch filter {
		case "", "all":
		case "failed":
			if !e.Failed {
				continue
			}
		case "xhr":
			if e.Type != "XHR" && e.Type != "Fetch" {
				continue
			}
		case "document":
			if e.Type != "Document" {
				continue
			}
		default:
			// 未知过滤条件按 all 处理。
		}
		out = append(out, *e)
	}
	if maxEntries > 0 && len(out) > maxEntries {
		out = out[len(out)-maxEntries:]
	}
	return out
}

// handleConsole 处理 Runtime.consoleAPICalled。
func (p *pageState) handleConsole(params json.RawMessage) {
	var ev struct {
		Type      string  `json:"type"`
		Timestamp float64 `json:"timestamp"`
		Args      []struct {
			Type        string          `json:"type"`
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"args"`
		StackTrace struct {
			CallFrames []struct {
				URL        string `json:"url"`
				LineNumber int    `json:"lineNumber"`
			} `json:"callFrames"`
		} `json:"stackTrace"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	var parts []string
	for _, arg := range ev.Args {
		if len(arg.Value) > 0 && string(arg.Value) != "null" {
			var s string
			if json.Unmarshal(arg.Value, &s) == nil {
				parts = append(parts, s)
			} else {
				parts = append(parts, string(arg.Value))
			}
			continue
		}
		if arg.Description != "" {
			parts = append(parts, arg.Description)
		}
	}
	url, line := "", 0
	if len(ev.StackTrace.CallFrames) > 0 {
		url = ev.StackTrace.CallFrames[0].URL
		line = ev.StackTrace.CallFrames[0].LineNumber
	}
	p.console.add(ConsoleEntry{
		Level: ev.Type,
		Text:  strings.Join(parts, " "),
		URL:   url,
		Line:  line,
		Time:  time.Now().Format(time.RFC3339),
	})
}

// handleException 处理 Runtime.exceptionThrown。
func (p *pageState) handleException(params json.RawMessage) {
	var ev struct {
		ExceptionDetails struct {
			Text      string `json:"text"`
			URL       string `json:"url"`
			LineNumber int   `json:"lineNumber"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	text := ev.ExceptionDetails.Text
	if ev.ExceptionDetails.Exception.Description != "" {
		text = ev.ExceptionDetails.Exception.Description
	}
	p.console.add(ConsoleEntry{
		Level: "error",
		Text:  text,
		URL:   ev.ExceptionDetails.URL,
		Line:  ev.ExceptionDetails.LineNumber,
		Time:  time.Now().Format(time.RFC3339),
	})
}

// handleLogEntry 处理 Log.entryAdded。
func (p *pageState) handleLogEntry(params json.RawMessage) {
	var ev struct {
		Entry struct {
			Level      string `json:"level"`
			Text       string `json:"text"`
			URL        string `json:"url"`
			LineNumber int    `json:"lineNumber"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(params, &ev); err != nil {
		return
	}
	p.console.add(ConsoleEntry{
		Level: ev.Entry.Level,
		Text:  ev.Entry.Text,
		URL:   ev.Entry.URL,
		Line:  ev.Entry.LineNumber,
		Time:  time.Now().Format(time.RFC3339),
	})
}
