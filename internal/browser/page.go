package browser

// 页面：标签页的创建、切换、关闭，导航和页面信息。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// pageState 是一个标签页的状态。
type pageState struct {
	targetID  string
	sessionID string

	mu         sync.Mutex
	url        string
	title      string
	refs       map[string]int64 // ref → backendNodeId
	snapshotID string
	console    *consoleBuffer
	network    *networkBuffer
}

// newPageState 创建页面状态。
func newPageState(targetID, sessionID string) *pageState {
	return &pageState{
		targetID:  targetID,
		sessionID: sessionID,
		refs:      make(map[string]int64),
		console:   newConsoleBuffer(1000),
		network:   newNetworkBuffer(1000),
	}
}

// setURLTitle 更新页面地址和标题。
func (p *pageState) setURLTitle(u, title string) {
	p.mu.Lock()
	p.url = u
	p.title = title
	p.mu.Unlock()
}

// info 返回页面地址和标题。
func (p *pageState) info() (string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.url, p.title
}

// resetSnapshot 清除快照引用；导航后旧 ref 失效。
func (p *pageState) resetSnapshot() {
	p.mu.Lock()
	p.refs = make(map[string]int64)
	p.snapshotID = ""
	p.mu.Unlock()
}

// backendForRef 返回 ref 对应的 backendNodeId。
func (p *pageState) backendForRef(ref string) (int64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id, ok := p.refs[ref]
	return id, ok
}

// createPage 创建一个新标签页并附加 CDP 会话。
func (s *Session) createPage(ctx context.Context, rawURL string) (*pageState, error) {
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.client.call(ctx, "", "Target.createTarget", map[string]any{"url": rawURL}, &created); err != nil {
		return nil, cdpProtocolError(err)
	}
	if created.TargetID == "" {
		return nil, protocol.Errorf(protocol.CodeInternal, "浏览器没有返回新标签页的 targetId")
	}
	return s.attachTarget(ctx, created.TargetID)
}

// attachTarget 附加到一个标签页；已经附加时直接返回。
func (s *Session) attachTarget(ctx context.Context, targetID string) (*pageState, error) {
	s.mu.Lock()
	p := s.pages[targetID]
	s.mu.Unlock()
	if p != nil {
		return p, nil
	}
	var att struct {
		SessionID string `json:"sessionId"`
	}
	if err := s.client.call(ctx, "", "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	}, &att); err != nil {
		return nil, cdpProtocolError(err)
	}
	if att.SessionID == "" {
		return nil, protocol.Errorf(protocol.CodeInternal, "附加到标签页失败：没有返回 sessionId")
	}
	p = newPageState(targetID, att.SessionID)
	s.mu.Lock()
	s.pages[targetID] = p
	s.bySession[att.SessionID] = p
	if s.current == "" {
		s.current = targetID
	}
	s.mu.Unlock()

	// Page、Runtime、DOM、Network 和 Accessibility 是快照和操作必需的；
	// Log 只用于收集控制台日志，失败不影响使用。
	required := []string{"Page.enable", "Runtime.enable", "DOM.enable", "Network.enable", "Accessibility.enable"}
	for _, method := range required {
		if err := s.client.call(ctx, att.SessionID, method, nil, nil); err != nil {
			return nil, cdpProtocolError(err)
		}
	}
	_ = s.client.call(ctx, att.SessionID, "Log.enable", nil, nil)
	_ = s.client.call(ctx, att.SessionID, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width":             s.viewport.Width,
		"height":            s.viewport.Height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	}, nil)
	s.refreshPageInfo(ctx, p)
	return p, nil
}

// setupEvents 注册 CDP 事件处理。
func (s *Session) setupEvents() {
	s.client.on("Runtime.consoleAPICalled", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.handleConsole(params) })
	})
	s.client.on("Runtime.exceptionThrown", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.handleException(params) })
	})
	s.client.on("Log.entryAdded", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.handleLogEntry(params) })
	})
	s.client.on("Network.requestWillBeSent", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.network.requestWillBeSent(params) })
	})
	s.client.on("Network.responseReceived", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.network.responseReceived(params) })
	})
	s.client.on("Network.loadingFinished", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.network.loadingFinished(params) })
	})
	s.client.on("Network.loadingFailed", func(sessionID string, params json.RawMessage) {
		s.withPage(sessionID, func(p *pageState) { p.network.loadingFailed(params) })
	})
	s.client.on("Page.frameNavigated", func(sessionID string, params json.RawMessage) {
		var ev struct {
			Frame struct {
				URL string `json:"url"`
			} `json:"frame"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}
		if ev.Type != "" && ev.Type != "Navigation" {
			return
		}
		s.withPage(sessionID, func(p *pageState) {
			if ev.Frame.URL != "" {
				p.setURLTitle(ev.Frame.URL, "")
				p.resetSnapshot()
			}
		})
	})
}

// refreshPageInfo 读取页面地址和标题。
func (s *Session) refreshPageInfo(ctx context.Context, p *pageState) {
	raw, err := s.evalValue(ctx, p.sessionID, `({url: location.href, title: document.title})`, false)
	if err != nil {
		return
	}
	var v struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if json.Unmarshal(raw, &v) == nil {
		p.setURLTitle(v.URL, v.Title)
	}
}

// UpdatePageInfo 刷新当前页面的地址和标题。
func (s *Session) updateCurrentPageInfo(ctx context.Context) {
	if p, err := s.currentPage(); err == nil {
		s.refreshPageInfo(ctx, p)
	}
}

// NavigateResult 是 browser.navigate 的返回值。
type NavigateResult struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Status  int    `json:"status,omitempty"`
	WaitFor string `json:"wait_until"`
}

// Navigate 打开一个 URL 并等待页面就绪。
//
// waitUntil 取值：load（默认）、domcontentloaded、network_idle。
func (s *Session) Navigate(ctx context.Context, rawURL, waitUntil string) (NavigateResult, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return NavigateResult{}, perr
	}
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return NavigateResult{}, protocol.Errorf(protocol.CodeInvalidArgs,
			"URL 不合法（需要带 scheme，如 https://example.com）：%q", rawURL)
	}
	scheme := strings.ToLower(u.Scheme)
	if !s.cfg.schemeAllowed(scheme) {
		return NavigateResult{}, protocol.Errorf(protocol.CodePermissionDenied,
			"不允许打开 %s: 链接（允许的 scheme：%s）", scheme, strings.Join(s.cfg.AllowedSchemes, "、"))
	}
	switch waitUntil {
	case "", "load":
		waitUntil = "load"
	case "domcontentloaded", "network_idle":
	default:
		return NavigateResult{}, protocol.Errorf(protocol.CodeInvalidArgs,
			"wait_until 只能是 load、domcontentloaded 或 network_idle")
	}

	var nav struct {
		FrameID   string `json:"frameId"`
		LoaderID  string `json:"loaderId"`
		ErrorText string `json:"errorText"`
	}
	if err := s.client.call(ctx, p.sessionID, "Page.navigate", map[string]any{"url": rawURL}, &nav); err != nil {
		return NavigateResult{}, cdpProtocolError(err)
	}
	if nav.ErrorText != "" {
		return NavigateResult{}, protocol.Errorf(protocol.CodeUnavailable, "打开页面失败：%s", nav.ErrorText)
	}
	p.resetSnapshot()
	if err := s.waitPageReady(ctx, p, waitUntil); err != nil {
		return NavigateResult{}, err
	}
	s.refreshPageInfo(ctx, p)
	url, title := p.info()
	return NavigateResult{URL: url, Title: title, WaitFor: waitUntil}, nil
}

// waitPageReady 等待页面满足条件。返回 ctx.Err() 以便节点层映射为 timeout。
func (s *Session) waitPageReady(ctx context.Context, p *pageState, waitUntil string) error {
	// 给导航一点时间提交，避免读到上一页的 readyState。
	p.resetSnapshot()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ready, err := s.readyState(ctx, p.sessionID)
			if err != nil {
				continue
			}
			switch waitUntil {
			case "domcontentloaded":
				if ready == "interactive" || ready == "complete" {
					return nil
				}
			case "network_idle":
				if ready == "complete" && p.network.idleFor(500*time.Millisecond) {
					return nil
				}
			default:
				if ready == "complete" {
					return nil
				}
			}
		}
	}
}

// readyState 读取 document.readyState。
func (s *Session) readyState(ctx context.Context, sessionID string) (string, error) {
	res, err := s.evalValue(ctx, sessionID, `document.readyState`, false)
	if err != nil {
		return "", err
	}
	var state string
	if json.Unmarshal(res, &state) != nil {
		return "", fmt.Errorf("无法解析 readyState")
	}
	return state, nil
}

// Tab 是一个标签页。
type Tab struct {
	Index    int    `json:"index"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Active   bool   `json:"active"`
	Attached bool   `json:"attached"`
}

// Tabs 列出所有标签页。
func (s *Session) Tabs(ctx context.Context) ([]Tab, error) {
	var res struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
			Title    string `json:"title"`
			Attached bool   `json:"attached"`
		} `json:"targetInfos"`
	}
	if err := s.client.call(ctx, "", "Target.getTargets", nil, &res); err != nil {
		return nil, cdpProtocolError(err)
	}
	s.mu.Lock()
	current := s.current
	attached := make(map[string]*pageState, len(s.pages))
	for id, p := range s.pages {
		attached[id] = p
	}
	s.mu.Unlock()
	out := make([]Tab, 0, len(res.TargetInfos))
	idx := 0
	for _, t := range res.TargetInfos {
		if t.Type != "page" {
			continue
		}
		_, ok := attached[t.TargetID]
		out = append(out, Tab{
			Index:    idx,
			URL:      t.URL,
			Title:    t.Title,
			Active:   t.TargetID == current,
			Attached: ok,
		})
		idx++
	}
	return out, nil
}

// SelectTab 切换当前标签页。
func (s *Session) SelectTab(ctx context.Context, index int) ([]Tab, error) {
	tabs, err := s.Tabs(ctx)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(tabs) {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "标签页序号超出范围：%d（共 %d 个）", index, len(tabs))
	}
	var res struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := s.client.call(ctx, "", "Target.getTargets", nil, &res); err != nil {
		return nil, cdpProtocolError(err)
	}
	var targetID string
	idx := 0
	for _, t := range res.TargetInfos {
		if t.Type != "page" {
			continue
		}
		if idx == index {
			targetID = t.TargetID
			break
		}
		idx++
	}
	if targetID == "" {
		return nil, protocol.Errorf(protocol.CodeNotFound, "找不到标签页 %d", index)
	}
	if _, err := s.attachTarget(ctx, targetID); err != nil {
		return nil, err
	}
	if err := s.client.call(ctx, "", "Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return nil, cdpProtocolError(err)
	}
	s.mu.Lock()
	s.current = targetID
	s.mu.Unlock()
	return s.Tabs(ctx)
}

// NewTab 新建标签页并切换过去。
func (s *Session) NewTab(ctx context.Context, rawURL string) ([]Tab, error) {
	if rawURL == "" {
		rawURL = "about:blank"
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "URL 不合法：%q", rawURL)
	}
	if !s.cfg.schemeAllowed(strings.ToLower(u.Scheme)) {
		return nil, protocol.Errorf(protocol.CodePermissionDenied, "不允许打开 %s: 链接", u.Scheme)
	}
	p, err := s.createPage(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if err := s.client.call(ctx, "", "Target.activateTarget", map[string]any{"targetId": p.targetID}, nil); err != nil {
		return nil, cdpProtocolError(err)
	}
	s.mu.Lock()
	s.current = p.targetID
	s.mu.Unlock()
	return s.Tabs(ctx)
}

// CloseTab 关闭标签页。
func (s *Session) CloseTab(ctx context.Context, index int) ([]Tab, error) {
	tabs, err := s.Tabs(ctx)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(tabs) {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "标签页序号超出范围：%d（共 %d 个）", index, len(tabs))
	}
	// Tabs 的顺序和 Target.getTargets 的 page 顺序一致，按同样方式取 targetId。
	var res struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if err := s.client.call(ctx, "", "Target.getTargets", nil, &res); err != nil {
		return nil, cdpProtocolError(err)
	}
	var targetID string
	idx := 0
	for _, t := range res.TargetInfos {
		if t.Type != "page" {
			continue
		}
		if idx == index {
			targetID = t.TargetID
			break
		}
		idx++
	}
	if targetID == "" {
		return nil, protocol.Errorf(protocol.CodeNotFound, "找不到标签页 %d", index)
	}
	if err := s.client.call(ctx, "", "Target.closeTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return nil, cdpProtocolError(err)
	}
	s.mu.Lock()
	if p := s.pages[targetID]; p != nil {
		delete(s.bySession, p.sessionID)
		delete(s.pages, targetID)
	}
	if s.current == targetID {
		s.current = ""
		for id := range s.pages {
			s.current = id
			break
		}
	}
	empty := s.current == ""
	s.mu.Unlock()
	if empty {
		// 没有标签页了，新建一个空白页，保证会话可以继续用。
		if _, err := s.createPage(ctx, "about:blank"); err != nil {
			return nil, err
		}
	}
	return s.Tabs(ctx)
}
