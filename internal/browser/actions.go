package browser

// 页面操作：执行 JS、点击、输入、按键、等待、截图、Cookie。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// Point 是页面内坐标（CSS 像素）。
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// evalResult 是 Runtime.evaluate 的返回。
type evalResult struct {
	Result struct {
		Type        string          `json:"type"`
		Value       json.RawMessage `json:"value"`
		Description string          `json:"description"`
	} `json:"result"`
	ExceptionDetails *struct {
		Text      string `json:"text"`
		Exception *struct {
			Description string `json:"description"`
		} `json:"exception"`
	} `json:"exceptionDetails"`
}

// evalValue 在一个页面里执行表达式并返回原始 JSON 值。
func (s *Session) evalValue(ctx context.Context, sessionID, expression string, awaitPromise bool) (json.RawMessage, error) {
	var res evalResult
	params := map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
		"userGesture":   true,
	}
	if err := s.client.call(ctx, sessionID, "Runtime.evaluate", params, &res); err != nil {
		return nil, cdpProtocolError(err)
	}
	if res.ExceptionDetails != nil {
		text := res.ExceptionDetails.Text
		if res.ExceptionDetails.Exception != nil && res.ExceptionDetails.Exception.Description != "" {
			text = res.ExceptionDetails.Exception.Description
		}
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "页面脚本抛出异常：%s", text)
	}
	if len(res.Result.Value) == 0 || res.Result.Type == "undefined" {
		return json.RawMessage("null"), nil
	}
	return res.Result.Value, nil
}

// Eval 执行一段 JS 并返回结果。
func (s *Session) Eval(ctx context.Context, expression string, awaitPromise bool, maxBytes int) (any, bool, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return nil, false, perr
	}
	raw, err := s.evalValue(ctx, p.sessionID, expression, awaitPromise)
	if err != nil {
		return nil, false, err
	}
	if len(raw) > maxBytes {
		preview := string(raw)
		if len(preview) > maxBytes {
			preview = preview[:maxBytes]
		}
		return map[string]any{"truncated": true, "size": len(raw), "preview": preview}, true, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw), false, nil
	}
	return v, false, nil
}

// Click 点击一个元素或坐标。
type ClickOptions struct {
	Ref        string
	Selector   string
	Point      *Point
	Button     string // left、right、middle
	ClickCount int
	Modifiers  []string
}

// Click 点击元素或坐标。
func (s *Session) Click(ctx context.Context, opts ClickOptions) error {
	p, perr := s.currentPage()
	if perr != nil {
		return perr
	}
	pt, err := s.resolvePoint(ctx, p, opts.Ref, opts.Selector, opts.Point)
	if err != nil {
		return err
	}
	button := opts.Button
	if button == "" {
		button = "left"
	}
	switch button {
	case "left", "right", "middle":
	default:
		return protocol.Errorf(protocol.CodeInvalidArgs, "button 只能是 left、right 或 middle")
	}
	count := opts.ClickCount
	if count <= 0 {
		count = 1
	}
	if count > 3 {
		return protocol.Errorf(protocol.CodeInvalidArgs, "click_count 不能超过 3")
	}
	mods := modifiersMask(opts.Modifiers)
	base := map[string]any{
		"x": pt.X, "y": pt.Y, "button": button, "modifiers": mods,
	}
	buttons := 1
	if button == "right" {
		buttons = 2
	} else if button == "middle" {
		buttons = 4
	}
	move := cloneMap(base)
	move["type"] = "mouseMoved"
	move["buttons"] = 0
	if err := s.client.call(ctx, p.sessionID, "Input.dispatchMouseEvent", move, nil); err != nil {
		return cdpProtocolError(err)
	}
	down := cloneMap(base)
	down["type"] = "mousePressed"
	down["clickCount"] = count
	down["buttons"] = buttons
	if err := s.client.call(ctx, p.sessionID, "Input.dispatchMouseEvent", down, nil); err != nil {
		return cdpProtocolError(err)
	}
	up := cloneMap(base)
	up["type"] = "mouseReleased"
	up["clickCount"] = count
	up["buttons"] = 0
	if err := s.client.call(ctx, p.sessionID, "Input.dispatchMouseEvent", up, nil); err != nil {
		return cdpProtocolError(err)
	}
	time.Sleep(100 * time.Millisecond)
	p.resetSnapshot()
	return nil
}

// cloneMap 复制一个 map。
func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// resolvePoint 把 ref、selector 或坐标解析成页面坐标。
func (s *Session) resolvePoint(ctx context.Context, p *pageState, ref, selector string, pt *Point) (Point, error) {
	if pt != nil {
		if pt.X < 0 || pt.Y < 0 {
			return Point{}, protocol.Errorf(protocol.CodeInvalidArgs, "坐标不能为负数")
		}
		return *pt, nil
	}
	if ref != "" {
		backend, ok := p.backendForRef(ref)
		if !ok {
			return Point{}, protocol.Errorf(protocol.CodeInvalidArgs,
				"ref %q 不存在或已失效，先调用 browser.snapshot 重新获取", ref)
		}
		if err := s.client.call(ctx, p.sessionID, "DOM.scrollIntoViewIfNeeded",
			map[string]any{"backendNodeId": backend}, nil); err != nil {
			return Point{}, cdpProtocolError(err)
		}
		var box struct {
			Model struct {
				Content []float64 `json:"content"`
				Width   float64   `json:"width"`
				Height  float64   `json:"height"`
			} `json:"model"`
		}
		if err := s.client.call(ctx, p.sessionID, "DOM.getBoxModel",
			map[string]any{"backendNodeId": backend}, &box); err != nil {
			return Point{}, protocol.Errorf(protocol.CodeInvalidArgs,
				"元素 %s 没有可见的布局（可能被隐藏或尺寸为 0）", ref)
		}
		return quadCenter(box.Model.Content)
	}
	if selector != "" {
		expr := fmt.Sprintf(`(() => {
  const el = document.querySelector(%s);
  if (!el) return null;
  el.scrollIntoView({block: "center", inline: "center"});
  const r = el.getBoundingClientRect();
  if (r.width <= 0 || r.height <= 0) return null;
  return {x: r.x + r.width / 2, y: r.y + r.height / 2};
})()`, jsString(selector))
		raw, err := s.evalValue(ctx, p.sessionID, expr, false)
		if err != nil {
			return Point{}, err
		}
		var pt Point
		if string(raw) == "null" || json.Unmarshal(raw, &pt) != nil || (pt.X == 0 && pt.Y == 0) {
			return Point{}, protocol.Errorf(protocol.CodeNotFound, "找不到可见的元素：%s", selector)
		}
		return pt, nil
	}
	return Point{}, protocol.Errorf(protocol.CodeInvalidArgs, "需要指定 ref、selector 或 point")
}

// quadCenter 计算内容区四边形（8 个坐标）的中心。
func quadCenter(quad []float64) (Point, error) {
	if len(quad) < 8 {
		return Point{}, protocol.Errorf(protocol.CodeInvalidArgs, "元素没有可见的布局")
	}
	x := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	y := (quad[1] + quad[3] + quad[5] + quad[7]) / 4
	return Point{X: x, Y: y}, nil
}

// jsString 把字符串编码成 JS 字面量。
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// modifiersMask 把修饰键列表转换成 CDP 的位掩码。
func modifiersMask(mods []string) int {
	mask := 0
	for _, m := range mods {
		switch strings.ToLower(m) {
		case "alt":
			mask |= 1
		case "ctrl", "control":
			mask |= 2
		case "meta", "cmd", "command":
			mask |= 4
		case "shift":
			mask |= 8
		}
	}
	return mask
}

// Type 聚焦元素并输入文本。
func (s *Session) Type(ctx context.Context, ref, selector, text string, clear, submit bool) (int, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return 0, perr
	}
	if ref == "" && selector == "" {
		return 0, protocol.Errorf(protocol.CodeInvalidArgs, "需要指定 ref 或 selector")
	}
	if err := s.focusElement(ctx, p, ref, selector); err != nil {
		return 0, err
	}
	if clear {
		if err := s.clearElement(ctx, p, ref, selector); err != nil {
			return 0, err
		}
	}
	if text != "" {
		if err := s.client.call(ctx, p.sessionID, "Input.insertText",
			map[string]any{"text": text}, nil); err != nil {
			return 0, cdpProtocolError(err)
		}
	}
	if submit {
		if err := s.Key(ctx, "Enter", nil); err != nil {
			return 0, err
		}
	}
	return len([]rune(text)), nil
}

// focusElement 让元素获得焦点。
func (s *Session) focusElement(ctx context.Context, p *pageState, ref, selector string) error {
	if ref != "" {
		backend, ok := p.backendForRef(ref)
		if !ok {
			return protocol.Errorf(protocol.CodeInvalidArgs,
				"ref %q 不存在或已失效，先调用 browser.snapshot 重新获取", ref)
		}
		if err := s.client.call(ctx, p.sessionID, "DOM.focus",
			map[string]any{"backendNodeId": backend}, nil); err != nil {
			return protocol.Errorf(protocol.CodeInvalidArgs, "元素无法获得焦点：%v", err)
		}
		return nil
	}
	expr := fmt.Sprintf(`(() => {
  const el = document.querySelector(%s);
  if (!el) return false;
  el.focus();
  return true;
})()`, jsString(selector))
	raw, err := s.evalValue(ctx, p.sessionID, expr, false)
	if err != nil {
		return err
	}
	if string(raw) != "true" {
		return protocol.Errorf(protocol.CodeNotFound, "找不到元素：%s", selector)
	}
	return nil
}

// clearElement 清空元素内容。对 input/textarea 设置 value 并触发 input 事件，
// 其他元素（contenteditable）清空 textContent。
func (s *Session) clearElement(ctx context.Context, p *pageState, ref, selector string) error {
	fn := `function () {
  if ("value" in this && typeof this.value === "string") {
    this.value = "";
    this.dispatchEvent(new Event("input", {bubbles: true}));
    this.dispatchEvent(new Event("change", {bubbles: true}));
  } else {
    this.textContent = "";
  }
  return true;
}`
	if selector != "" {
		expr := fmt.Sprintf(`(() => {
  const el = document.querySelector(%s);
  if (!el) return false;
  if ("value" in el && typeof el.value === "string") {
    el.value = "";
    el.dispatchEvent(new Event("input", {bubbles: true}));
    el.dispatchEvent(new Event("change", {bubbles: true}));
  } else {
    el.textContent = "";
  }
  return true;
})()`, jsString(selector))
		raw, err := s.evalValue(ctx, p.sessionID, expr, false)
		if err != nil {
			return err
		}
		if string(raw) != "true" {
			return protocol.Errorf(protocol.CodeNotFound, "找不到元素：%s", selector)
		}
		return nil
	}
	backend, ok := p.backendForRef(ref)
	if !ok {
		return protocol.Errorf(protocol.CodeInvalidArgs, "ref 不存在或已失效")
	}
	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := s.client.call(ctx, p.sessionID, "DOM.resolveNode",
		map[string]any{"backendNodeId": backend}, &resolved); err != nil {
		return cdpProtocolError(err)
	}
	return s.client.call(ctx, p.sessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId":            resolved.Object.ObjectID,
		"functionDeclaration": fn,
		"returnByValue":       true,
	}, nil)
}

// keyInfo 是按键的 CDP 参数。
type keyInfo struct {
	Key       string
	Code      string
	VK        int
	Text      string
	KeyCode32 int
}

// keyTable 是支持的命名按键。
var keyTable = map[string]keyInfo{
	"Enter":      {"Enter", "Enter", 13, "\r", 13},
	"Tab":        {"Tab", "Tab", 9, "", 9},
	"Escape":     {"Escape", "Escape", 27, "", 27},
	"Esc":        {"Escape", "Escape", 27, "", 27},
	"Backspace":  {"Backspace", "Backspace", 8, "", 8},
	"Delete":     {"Delete", "Delete", 46, "", 46},
	"ArrowUp":    {"ArrowUp", "ArrowUp", 38, "", 38},
	"ArrowDown":  {"ArrowDown", "ArrowDown", 40, "", 40},
	"ArrowLeft":  {"ArrowLeft", "ArrowLeft", 37, "", 37},
	"ArrowRight": {"ArrowRight", "ArrowRight", 39, "", 39},
	"Home":       {"Home", "Home", 36, "", 36},
	"End":        {"End", "End", 35, "", 35},
	"PageUp":     {"PageUp", "PageUp", 33, "", 33},
	"PageDown":   {"PageDown", "PageDown", 34, "", 34},
	"Space":      {" ", "Space", 32, " ", 32},
	" ":          {" ", "Space", 32, " ", 32},
}

// Key 按下并释放一个键。
func (s *Session) Key(ctx context.Context, key string, mods []string) error {
	p, perr := s.currentPage()
	if perr != nil {
		return perr
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return protocol.Errorf(protocol.CodeInvalidArgs, "key 不能为空")
	}
	mask := modifiersMask(mods)
	info, ok := keyTable[key]
	if !ok && len([]rune(key)) == 1 {
		r := []rune(key)[0]
		upper := strings.ToUpper(key)
		code := "Key" + upper
		if r >= '0' && r <= '9' {
			code = "Digit" + upper
		}
		info = keyInfo{Key: key, Code: code, VK: int(upper[0]), Text: key}
		ok = true
	}
	if !ok {
		return protocol.Errorf(protocol.CodeInvalidArgs,
			"不支持的按键：%s（支持 Enter、Tab、Escape、方向键、Delete、Backspace、字母数字等）", key)
	}
	down := map[string]any{
		"type":                  "keyDown",
		"key":                   info.Key,
		"code":                  info.Code,
		"windowsVirtualKeyCode": info.VK,
		"nativeVirtualKeyCode":  info.VK,
		"modifiers":             mask,
	}
	if info.Text != "" {
		down["text"] = info.Text
		down["unmodifiedText"] = info.Text
	}
	if err := s.client.call(ctx, p.sessionID, "Input.dispatchKeyEvent", down, nil); err != nil {
		return cdpProtocolError(err)
	}
	up := cloneMap(down)
	up["type"] = "keyUp"
	delete(up, "text")
	delete(up, "unmodifiedText")
	if err := s.client.call(ctx, p.sessionID, "Input.dispatchKeyEvent", up, nil); err != nil {
		return cdpProtocolError(err)
	}
	p.resetSnapshot()
	return nil
}

// Wait 等待页面满足条件。
func (s *Session) Wait(ctx context.Context, selector, text, rawURL, state string) (int64, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return 0, perr
	}
	if selector == "" && text == "" && rawURL == "" {
		return 0, protocol.Errorf(protocol.CodeInvalidArgs, "需要指定 selector、text 或 url")
	}
	if state == "" {
		state = "visible"
	}
	switch state {
	case "visible", "attached":
	default:
		return 0, protocol.Errorf(protocol.CodeInvalidArgs, "state 只能是 visible 或 attached")
	}
	start := time.Now()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
			if selector != "" {
				ok, err := s.selectorMatches(ctx, p, selector, state)
				if err != nil {
					return 0, err
				}
				if ok {
					return time.Since(start).Milliseconds(), nil
				}
			}
			if text != "" {
				ok, err := s.pageContainsText(ctx, p, text)
				if err != nil {
					return 0, err
				}
				if ok {
					return time.Since(start).Milliseconds(), nil
				}
			}
			if rawURL != "" {
				ok, err := s.pageURLContains(ctx, p, rawURL)
				if err != nil {
					return 0, err
				}
				if ok {
					return time.Since(start).Milliseconds(), nil
				}
			}
		}
	}
}

// selectorMatches 判断选择器匹配的元素是否满足状态。
func (s *Session) selectorMatches(ctx context.Context, p *pageState, selector, state string) (bool, error) {
	expr := fmt.Sprintf(`(() => {
  const el = document.querySelector(%s);
  if (!el) return false;
  if (%q === "attached") return true;
  const r = el.getBoundingClientRect();
  const style = getComputedStyle(el);
  return r.width > 0 && r.height > 0 && style.visibility !== "hidden" && style.display !== "none";
})()`, jsString(selector), state)
	raw, err := s.evalValue(ctx, p.sessionID, expr, false)
	if err != nil {
		return false, err
	}
	return string(raw) == "true", nil
}

// pageContainsText 判断页面文本是否包含指定内容。
func (s *Session) pageContainsText(ctx context.Context, p *pageState, text string) (bool, error) {
	expr := fmt.Sprintf(`(document.body ? document.body.innerText : "").includes(%s)`, jsString(text))
	raw, err := s.evalValue(ctx, p.sessionID, expr, false)
	if err != nil {
		return false, err
	}
	return string(raw) == "true", nil
}

// pageURLContains 判断当前地址是否包含指定内容。
func (s *Session) pageURLContains(ctx context.Context, p *pageState, part string) (bool, error) {
	expr := fmt.Sprintf(`location.href.includes(%s)`, jsString(part))
	raw, err := s.evalValue(ctx, p.sessionID, expr, false)
	if err != nil {
		return false, err
	}
	return string(raw) == "true", nil
}

// screenshotResult 是 browser.screenshot 的返回值。
type screenshotResult struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
}

// Screenshot 截取页面并保存到文件。
func (s *Session) Screenshot(ctx context.Context, fullPage bool, format string, quality int, clip *Clip, savePath string) (screenshotResult, error) {
	p, perr := s.currentPage()
	if perr != nil {
		return screenshotResult{}, perr
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "png"
	}
	switch format {
	case "png", "jpeg", "jpg":
		if format == "jpg" {
			format = "jpeg"
		}
	default:
		return screenshotResult{}, protocol.Errorf(protocol.CodeInvalidArgs, "format 只能是 png 或 jpeg")
	}
	params := map[string]any{"format": format}
	if format == "jpeg" && quality > 0 {
		if quality > 100 {
			quality = 100
		}
		params["quality"] = quality
	}
	var width, height int
	if clip != nil {
		params["clip"] = map[string]any{
			"x": clip.X, "y": clip.Y, "width": clip.Width, "height": clip.Height, "scale": 1,
		}
		width, height = int(clip.Width), int(clip.Height)
	}
	if fullPage {
		cw, ch, err := s.contentSize(ctx, p)
		if err != nil {
			return screenshotResult{}, err
		}
		height = int(ch)
		if height > 20000 {
			height = 20000
		}
		params["captureBeyondViewport"] = true
		params["clip"] = map[string]any{
			"x": 0, "y": 0, "width": cw, "height": float64(height), "scale": 1,
		}
		width = int(cw)
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := s.client.call(ctx, p.sessionID, "Page.captureScreenshot", params, &res); err != nil {
		return screenshotResult{}, cdpProtocolError(err)
	}
	data, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return screenshotResult{}, protocol.Errorf(protocol.CodeInternal, "解码截图失败：%v", err)
	}
	path := savePath
	if path == "" {
		dir := s.cfg.screenshotDir(s.home)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return screenshotResult{}, protocol.Errorf(protocol.CodeInternal, "创建截图目录失败：%v", err)
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.%s", s.id, time.Now().UnixNano(), format))
	} else {
		path = expandUserPath(path)
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.home, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return screenshotResult{}, protocol.Errorf(protocol.CodeInvalidArgs, "创建截图目录失败：%v", err)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return screenshotResult{}, protocol.Errorf(protocol.CodeInvalidArgs, "写入截图失败：%v", err)
	}
	return screenshotResult{Path: path, Width: width, Height: height, Bytes: len(data)}, nil
}

// Clip 是截图区域。
type Clip struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// contentSize 返回页面内容尺寸（CSS 像素）。
func (s *Session) contentSize(ctx context.Context, p *pageState) (float64, float64, error) {
	var metrics struct {
		CSSContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"cssContentSize"`
		ContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"contentSize"`
	}
	if err := s.client.call(ctx, p.sessionID, "Page.getLayoutMetrics", nil, &metrics); err != nil {
		return 0, 0, cdpProtocolError(err)
	}
	w, h := metrics.CSSContentSize.Width, metrics.CSSContentSize.Height
	if w == 0 || h == 0 {
		w, h = metrics.ContentSize.Width, metrics.ContentSize.Height
	}
	if w <= 0 || h <= 0 {
		return 0, 0, protocol.Errorf(protocol.CodeInternal, "取不到页面内容尺寸")
	}
	return w, h, nil
}

// Cookie 是一条 Cookie。
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"http_only,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"same_site,omitempty"`
}

// Cookies 返回浏览器里的全部 Cookie。
func (s *Session) Cookies(ctx context.Context) ([]Cookie, error) {
	var res struct {
		Cookies []Cookie `json:"cookies"`
	}
	if err := s.client.call(ctx, "", "Network.getAllCookies", nil, &res); err != nil {
		return nil, cdpProtocolError(err)
	}
	return res.Cookies, nil
}

// SetCookies 设置一批 Cookie。
func (s *Session) SetCookies(ctx context.Context, rawURL string, cookies []Cookie) error {
	if rawURL == "" {
		return protocol.Errorf(protocol.CodeInvalidArgs, "设置 Cookie 需要 url")
	}
	for _, c := range cookies {
		if c.Name == "" {
			return protocol.Errorf(protocol.CodeInvalidArgs, "Cookie 缺少 name")
		}
		params := map[string]any{
			"name":  c.Name,
			"value": c.Value,
			"url":   rawURL,
		}
		if c.Domain != "" {
			params["domain"] = c.Domain
		}
		if c.Path != "" {
			params["path"] = c.Path
		}
		if c.HTTPOnly {
			params["httpOnly"] = true
		}
		if c.Secure {
			params["secure"] = true
		}
		if c.Expires > 0 {
			params["expires"] = c.Expires
		}
		if c.SameSite != "" {
			params["sameSite"] = c.SameSite
		}
		var res struct {
			Success bool `json:"success"`
		}
		if err := s.client.call(ctx, "", "Network.setCookie", params, &res); err != nil {
			return cdpProtocolError(err)
		}
	}
	return nil
}

// ClearCookies 清除浏览器里的全部 Cookie。
func (s *Session) ClearCookies(ctx context.Context) error {
	if err := s.client.call(ctx, "", "Network.clearBrowserCookies", nil, nil); err != nil {
		return cdpProtocolError(err)
	}
	return nil
}
