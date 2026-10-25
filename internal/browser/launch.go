package browser

// 启动浏览器进程：独立的用户数据目录、远程调试端口，等待 DevToolsActivePort
// 出现后返回浏览器级 WebSocket 地址。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// devToolsWait 是等待浏览器调试端口就绪的最长时间。
const devToolsWait = 20 * time.Second

// tailBuffer 保存浏览器输出末尾的一部分，启动失败时写进错误消息。
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.max <= 0 {
		t.max = 64 << 10
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// launched 是启动完成的浏览器进程。
type launched struct {
	cmd        *exec.Cmd
	wsURL      string
	version    string
	profileDir string
	isolated   bool
	output     *tailBuffer
	waitCh     chan error
}

// browserArgs 返回启动参数。
func browserArgs(profileDir string, headless bool) []string {
	args := []string{
		"--remote-debugging-port=0",
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-search-engine-choice-screen",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-extensions",
		"--disable-component-update",
		"--disable-default-apps",
		"--metrics-recording-only",
		"--no-pings",
		"--password-store=basic",
		"--use-mock-keychain",
	}
	if headless {
		args = append(args, "--headless=new")
	}
	return args
}

// launch 启动浏览器并等待 CDP 端点。
func launch(ctx context.Context, exe, profileDir string, isolated, headless bool) (*launched, error) {
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建浏览器配置目录 %s 失败：%w", profileDir, err)
	}
	cmd := exec.Command(exe, browserArgs(profileDir, headless)...)
	out := &tailBuffer{max: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = out
	prepareCmd(cmd)
	if err := cmd.Start(); err != nil {
		removeProfile(profileDir, isolated)
		return nil, fmt.Errorf("启动浏览器 %s 失败：%w", exe, err)
	}
	l := &launched{
		cmd:        cmd,
		profileDir: profileDir,
		isolated:   isolated,
		output:     out,
		waitCh:     make(chan error, 1),
	}
	go func() { l.waitCh <- cmd.Wait() }()
	assignToJob(cmd.Process.Pid)

	wsURL, err := waitDevTools(ctx, l)
	if err != nil {
		l.kill()
		l.wait(2 * time.Second)
		removeProfile(profileDir, isolated)
		return nil, fmt.Errorf("%w（浏览器输出：%s）", err, clip(out.String(), 600))
	}
	l.wsURL = wsURL
	l.version = fetchBrowserVersion(wsURL)
	return l, nil
}

// waitDevTools 等待 DevToolsActivePort 文件或输出里的 ws 地址。
func waitDevTools(ctx context.Context, l *launched) (string, error) {
	deadline := time.NewTimer(devToolsWait)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	portFile := filepath.Join(l.profileDir, "DevToolsActivePort")
	for {
		if ws := readDevToolsPort(portFile); ws != "" {
			return ws, nil
		}
		if ws := wsFromOutput(l.output.String()); ws != "" {
			return ws, nil
		}
		select {
		case err := <-l.waitCh:
			l.waitCh <- err // 放回去，kill/wait 还要用
			return "", fmt.Errorf("浏览器在调试端口就绪前退出：%v", err)
		case <-deadline.C:
			return "", fmt.Errorf("等待浏览器调试端口超时（%s）", devToolsWait)
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// readDevToolsPort 读取 DevToolsActivePort 文件并拼出浏览器级 WebSocket 地址。
func readDevToolsPort(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 2 {
		return ""
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port <= 0 {
		return ""
	}
	wsPath := strings.TrimSpace(lines[1])
	if wsPath == "" {
		return ""
	}
	if !strings.HasPrefix(wsPath, "/") {
		wsPath = "/" + wsPath
	}
	return fmt.Sprintf("ws://127.0.0.1:%d%s", port, wsPath)
}

// wsFromOutput 从浏览器输出里找 "DevTools listening on ws://..."。
func wsFromOutput(output string) string {
	const marker = "DevTools listening on "
	idx := strings.LastIndex(output, marker)
	if idx < 0 {
		return ""
	}
	rest := output[idx+len(marker):]
	if end := strings.IndexAny(rest, "\r\n"); end >= 0 {
		rest = rest[:end]
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "ws://") {
		return rest
	}
	return ""
}

// fetchBrowserVersion 读取 /json/version 里的浏览器版本；失败时返回空串。
func fetchBrowserVersion(wsURL string) string {
	u, err := httpURLFromWS(wsURL)
	if err != nil {
		return ""
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(u + "/json/version")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	var info struct {
		Browser string `json:"Browser"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return ""
	}
	return info.Browser
}

// httpURLFromWS 把 ws://127.0.0.1:port/devtools/browser/xxx 转成 http://127.0.0.1:port。
func httpURLFromWS(wsURL string) (string, error) {
	rest := strings.TrimPrefix(wsURL, "ws://")
	if rest == wsURL {
		return "", fmt.Errorf("不是 ws 地址：%s", wsURL)
	}
	host := rest
	if i := strings.Index(rest, "/"); i >= 0 {
		host = rest[:i]
	}
	return "http://" + host, nil
}

// kill 结束浏览器进程。
func (l *launched) kill() {
	if l == nil || l.cmd == nil || l.cmd.Process == nil {
		return
	}
	terminate(l.cmd)
}

// wait 等待浏览器进程退出，超时后强制结束。
func (l *launched) wait(timeout time.Duration) {
	if l == nil || l.cmd == nil {
		return
	}
	select {
	case <-l.waitCh:
	case <-time.After(timeout):
		terminate(l.cmd)
		select {
		case <-l.waitCh:
		case <-time.After(2 * time.Second):
		}
	}
}

// close 结束浏览器进程并清理临时配置目录。
func (l *launched) close() {
	if l == nil {
		return
	}
	l.kill()
	l.wait(3 * time.Second)
	removeProfile(l.profileDir, l.isolated)
}

// removeProfile 删除临时配置目录；persistent 目录保留。
func removeProfile(dir string, isolated bool) {
	if !isolated || dir == "" {
		return
	}
	// 浏览器退出时可能还有文件句柄未释放，Windows 上删除会失败，重试几次。
	for i := 0; i < 5; i++ {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// clip 截断字符串，避免错误消息过长。
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
