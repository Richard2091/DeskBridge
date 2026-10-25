package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOpenWithoutBrowser 验证找不到浏览器时返回可读的错误，不需要真实浏览器。
func TestOpenWithoutBrowser(t *testing.T) {
	home := t.TempDir()
	m, err := NewManager(home, t.Logf)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.CloseAll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, perr := m.Open(ctx, "test", OpenOptions{Executable: filepath.Join(home, "no-such-browser")})
	if perr == nil {
		t.Fatal("指定不存在的浏览器应返回错误")
	}
	if perr.Code != "invalid_args" && perr.Code != "unavailable" {
		t.Errorf("错误码 = %s，期望 invalid_args 或 unavailable（%s）", perr.Code, perr.Message)
	}
}

// TestBrowserE2E 需要真实 Chromium 浏览器，默认跳过：
//
//	DESKBRIDGE_BROWSER_TEST=1 DESKBRIDGE_BROWSER_EXECUTABLE="C:\...\chrome.exe" go test ./internal/browser -run TestBrowserE2E -v
func TestBrowserE2E(t *testing.T) {
	if os.Getenv("DESKBRIDGE_BROWSER_TEST") == "" {
		t.Skip("设置 DESKBRIDGE_BROWSER_TEST=1 启用真实浏览器测试")
	}
	home := t.TempDir()
	m, err := NewManager(home, t.Logf)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.CloseAll()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	headless := true
	exe := os.Getenv("DESKBRIDGE_BROWSER_EXECUTABLE")
	s, info, perr := m.Open(ctx, "test", OpenOptions{Headless: &headless, Executable: exe})
	if perr != nil {
		t.Fatalf("打开浏览器失败（%s）：%s", perr.Code, perr.Message)
	}
	t.Logf("浏览器：%s，默认浏览器=%v，方式=%s，警告=%v", info.Browser, info.DefaultBrowser, info.Method, info.Warnings)

	// 简单的测试页面，用 data URL 避免依赖网络。
	html := `<html><head><title>快照测试</title></head><body>
<button id="b">提交</button><input id="i"><div id="out"></div></body></html>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	nav, err := s.Navigate(ctx, dataURL, "load")
	if err != nil {
		t.Fatalf("导航失败：%v", err)
	}
	if nav.Title != "快照测试" {
		t.Errorf("标题 = %q，期望 快照测试", nav.Title)
	}

	snap, err := s.Snapshot(ctx, 0, 0)
	if err != nil {
		t.Fatalf("快照失败：%v", err)
	}
	var buttonRef string
	for _, n := range snap.Nodes {
		if n.Role == "button" && strings.Contains(n.Name, "提交") {
			buttonRef = n.Ref
			break
		}
	}
	if buttonRef == "" {
		t.Fatalf("快照里找不到按钮：%+v", snap.Nodes)
	}
	if err := s.Click(ctx, ClickOptions{Ref: buttonRef, SnapshotID: snap.SnapshotID}); err != nil {
		t.Fatalf("点击失败：%v", err)
	}

	if _, err := s.Type(ctx, "", "#i", "你好", false, false); err != nil {
		t.Fatalf("输入失败：%v", err)
	}
	val, _, err := s.Eval(ctx, `document.querySelector("#i").value`, false, 1024)
	if err != nil {
		t.Fatalf("执行 JS 失败：%v", err)
	}
	if fmt.Sprint(val) != "你好" {
		t.Errorf("输入框的值 = %v，期望 你好", val)
	}

	shot := filepath.Join(home, "shot.png")
	res, err := s.Screenshot(ctx, false, "png", 0, nil, shot)
	if err != nil {
		t.Fatalf("截图失败：%v", err)
	}
	if info, err := os.Stat(res.Path); err != nil || info.Size() == 0 {
		t.Errorf("截图为空：%v", err)
	}

	// 控制台消息。
	if _, _, err := s.Eval(ctx, `console.log("hello-console")`, false, 1024); err != nil {
		t.Fatalf("console.log 失败：%v", err)
	}
	time.Sleep(300 * time.Millisecond)
	entries, err := s.Console(ctx, "", 50, false)
	if err != nil {
		t.Fatalf("读取控制台失败：%v", err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Text, "hello-console") {
			found = true
		}
	}
	if !found {
		t.Errorf("控制台没有收到消息：%+v", entries)
	}

	// 标签页至少一个。
	tabs, err := s.Tabs(ctx)
	if err != nil {
		t.Fatalf("读取标签页失败：%v", err)
	}
	if len(tabs) == 0 {
		t.Error("标签页列表为空")
	}
}
