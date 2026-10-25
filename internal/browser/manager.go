package browser

// 管理器：管理一台设备上的浏览器会话。会话由调用方显式创建和关闭，
// 空闲超时后自动关闭；每个会话有独立的浏览器进程和配置档案目录。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// OpenOptions 是打开一个浏览器会话的参数。
type OpenOptions struct {
	Headless    *bool
	Executable  string
	Profile     string // isolated 或 persistent，空表示用配置
	ProfileName string
	Viewport    Viewport
}

// Viewport 是页面视口大小。
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// OpenInfo 是 browser.open 返回的信息。
type OpenInfo struct {
	Session        string   `json:"session"`
	Browser        string   `json:"browser"`
	Executable     string   `json:"executable"`
	Headless       bool     `json:"headless"`
	DefaultBrowser bool     `json:"default_browser"`
	Method         string   `json:"method"`
	Warnings       []string `json:"warnings,omitempty"`
}

// SessionInfo 是 browser.info 里一个会话的摘要。
type SessionInfo struct {
	Session       string `json:"session"`
	Browser       string `json:"browser"`
	Headless      bool   `json:"headless"`
	Pages         int    `json:"pages"`
	IdleSeconds   int    `json:"idle_seconds"`
	CreatedAtUnix int64  `json:"created_at_unix"`
}

// Manager 是浏览器会话集合。
type Manager struct {
	home string
	cfg  Config
	logf func(string, ...any)

	mu       sync.Mutex
	sessions map[string]*Session
	closed   bool

	stop chan struct{}
	wg   sync.WaitGroup
}

// NewManager 读取配置并创建管理器。配置文件不存在时使用默认配置。
func NewManager(home string, logf func(string, ...any)) (*Manager, error) {
	if logf == nil {
		logf = log.Printf
	}
	cfg, err := LoadConfig(home)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		home:     home,
		cfg:      cfg,
		logf:     logf,
		sessions: make(map[string]*Session),
		stop:     make(chan struct{}),
	}
	m.wg.Add(1)
	go m.reapLoop()
	return m, nil
}

// Config 返回当前配置。
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// SessionInfos 返回所有会话的摘要。
func (m *Manager) SessionInfos() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SessionInfo, 0, len(m.sessions))
	now := time.Now()
	for _, s := range m.sessions {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			continue
		}
		pages := len(s.pages)
		s.mu.Unlock()
		idle := int(now.Sub(time.Unix(0, s.lastUsed.Load())).Seconds())
		if idle < 0 {
			idle = 0
		}
		out = append(out, SessionInfo{
			Session:     s.id,
			Browser:     s.browserLabel(),
			Headless:    s.headless,
			Pages:       pages,
			IdleSeconds: idle,
			CreatedAtUnix: s.createdAt.Unix(),
		})
	}
	return out
}

// Open 启动一个浏览器会话。owner 是调用者身份，用于会话归属校验。
func (m *Manager) Open(ctx context.Context, owner string, opts OpenOptions) (*Session, OpenInfo, *protocol.Error) {
	m.mu.Lock()
	cfg := m.cfg
	closed := m.closed
	count := len(m.sessions)
	m.mu.Unlock()
	if closed {
		return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable, "节点正在退出，浏览器不可用")
	}
	if !cfg.Enabled {
		return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable,
			"浏览器工具已在 %s 中关闭（enabled 为 false）", ConfigPath(m.home))
	}
	if count >= cfg.MaxSessions {
		return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable,
			"同时打开的浏览器会话已达上限 %d，先关闭不用的会话", cfg.MaxSessions)
	}

	headless := cfg.Headless
	if opts.Headless != nil {
		headless = *opts.Headless
	}
	if opts.Profile == "isolated" || opts.Profile == "persistent" {
		cfg.ProfileMode = opts.Profile
	}
	viewport := opts.Viewport
	if viewport.Width <= 0 {
		viewport.Width = 1280
	}
	if viewport.Height <= 0 {
		viewport.Height = 800
	}

	exe := opts.Executable
	if exe == "" {
		exe = cfg.Executable
	}
	list, warnings, err := candidates(cfg, exe)
	if err != nil {
		return nil, OpenInfo{}, browserResolveError(err, warnings)
	}

	sessionID := randomID()
	profileDir := cfg.profileDir(m.home, sessionID, opts.ProfileName)
	isolated := cfg.ProfileMode != "persistent"
	if !isolated {
		// persistent 配置档案同一时间只能被一个会话使用，否则第二实例会把
		// 命令转发给已有实例，关闭时可能影响另一个会话。
		m.mu.Lock()
		for _, existing := range m.sessions {
			if existing.proc != nil && samePath(existing.proc.profileDir, profileDir) {
				m.mu.Unlock()
				return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable,
					"配置档案 %q 正在被另一个会话使用", opts.ProfileName)
			}
		}
		m.mu.Unlock()
	}

	var chosen Candidate
	var proc *launched
	var launchErr error
	for i, c := range list {
		if i >= 2 {
			break
		}
		if c.Method != "config" && c.Method != "fallback" {
			warnings = append(warnings, fmt.Sprintf("尝试默认浏览器 %s（%s）", c.Name, c.Path))
		}
		proc, launchErr = launch(ctx, c.Path, profileDir, isolated, headless)
		if launchErr == nil {
			chosen = c
			break
		}
		warnings = append(warnings, fmt.Sprintf("启动 %s 失败：%v", c.Name, launchErr))
		if isolated {
			removeProfile(profileDir, true)
		}
	}
	if proc == nil {
		return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable,
			"启动浏览器失败：%v", launchErr)
	}

	client, err := dialCDP(ctx, proc.wsURL)
	if err != nil {
		proc.close()
		return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable,
			"连接浏览器调试端口失败：%v", err)
	}

	s := &Session{
		id:           sessionID,
		owner:        owner,
		home:         m.home,
		cfg:          cfg,
		client:       client,
		proc:         proc,
		pages:        make(map[string]*pageState),
		bySession:    make(map[string]*pageState),
		headless:     headless,
		browserName:  chosen.Name,
		browserVer:   proc.version,
		viewport:     viewport,
		createdAt:    time.Now(),
	}
	s.lastUsed.Store(time.Now().UnixNano())
	s.setupEvents()

	if _, perr := s.openInitialPage(ctx); perr != nil {
		s.close()
		return nil, OpenInfo{}, perr
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		s.close()
		return nil, OpenInfo{}, protocol.Errorf(protocol.CodeUnavailable, "节点正在退出，浏览器不可用")
	}
	m.sessions[sessionID] = s
	m.mu.Unlock()
	m.logf("浏览器会话 %s 已启动：%s（%s，无头=%v）", sessionID, chosen.Path, proc.version, headless)

	return s, OpenInfo{
		Session:        sessionID,
		Browser:        s.browserLabel(),
		Executable:     chosen.Path,
		Headless:       headless,
		DefaultBrowser: chosen.Default,
		Method:         chosen.Method,
		Warnings:       warnings,
	}, nil
}

// browserResolveError 把浏览器选择错误转换成协议错误。
func browserResolveError(err error, warnings []string) *protocol.Error {
	msg := err.Error()
	if len(warnings) > 0 {
		msg += "（" + joinWarnings(warnings) + "）"
	}
	if err == ErrDefaultBrowserUnsupported {
		return protocol.Errorf(protocol.CodeUnsupportedPlatform, "%s", msg)
	}
	return protocol.Errorf(protocol.CodeUnavailable, "%s", msg)
}

// joinWarnings 把提示合并成一行。
func joinWarnings(warnings []string) string {
	out := ""
	for i, w := range warnings {
		if i > 0 {
			out += "；"
		}
		out += w
	}
	return out
}

// Get 按会话 id 取会话，并校验归属。
func (m *Manager) Get(owner, sessionID string) (*Session, *protocol.Error) {
	m.mu.Lock()
	s := m.sessions[sessionID]
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, protocol.Errorf(protocol.CodeUnavailable, "节点正在退出，浏览器不可用")
	}
	if s == nil {
		return nil, protocol.Errorf(protocol.CodeNotFound, "浏览器会话不存在：%s", sessionID)
	}
	s.mu.Lock()
	isClosed := s.closed
	s.mu.Unlock()
	if isClosed {
		return nil, protocol.Errorf(protocol.CodeNotFound, "浏览器会话已关闭：%s", sessionID)
	}
	if owner != "" && s.owner != owner {
		return nil, protocol.Errorf(protocol.CodePermissionDenied, "浏览器会话不属于当前调用者")
	}
	s.touch()
	return s, nil
}

// Close 关闭一个会话。
func (m *Manager) Close(owner, sessionID string) *protocol.Error {
	s, perr := m.Get(owner, sessionID)
	if perr != nil {
		return perr
	}
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	s.close()
	return nil
}

// CloseAll 关闭所有会话。节点退出时调用。
func (m *Manager) CloseAll() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.sessions = make(map[string]*Session)
	close(m.stop)
	m.mu.Unlock()
	m.wg.Wait()
	for _, s := range list {
		s.close()
	}
}

// reapLoop 定期关闭空闲超时的会话。
func (m *Manager) reapLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.reapIdle()
		}
	}
}

// reapIdle 关闭空闲超时的会话。
func (m *Manager) reapIdle() {
	cfg := m.Config()
	timeout := cfg.idleTimeout()
	if timeout <= 0 {
		return
	}
	now := time.Now()
	var idle []*Session
	m.mu.Lock()
	for id, s := range m.sessions {
		if now.Sub(time.Unix(0, s.lastUsed.Load())) >= timeout {
			idle = append(idle, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()
	for _, s := range idle {
		m.logf("浏览器会话 %s 空闲超过 %s，自动关闭", s.id, timeout)
		s.close()
	}
}

// randomID 生成 8 字节的十六进制会话 id。
func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Session 是一个浏览器会话：一个浏览器进程和若干标签页。
type Session struct {
	id    string
	owner string
	home  string
	cfg   Config

	mu        sync.Mutex
	client    *cdpClient
	proc      *launched
	pages     map[string]*pageState // targetID → 页面
	bySession map[string]*pageState // CDP sessionId → 页面
	current   string
	closed    bool

	headless       bool
	browserName    string
	browserVer     string
	defaultBrowser bool
	viewport       Viewport
	createdAt      time.Time
	lastUsed       atomic.Int64
}

// ID 返回会话 id。
func (s *Session) ID() string { return s.id }

// BrowserLabel 返回浏览器名字和版本。
func (s *Session) browserLabel() string {
	if s.browserVer == "" {
		return s.browserName
	}
	return s.browserName + " " + s.browserVer
}

// touch 记录最近使用时间。
func (s *Session) touch() {
	s.lastUsed.Store(time.Now().UnixNano())
}

// openInitialPage 创建一个空白页并设为当前页。
func (s *Session) openInitialPage(ctx context.Context) (*pageState, *protocol.Error) {
	p, err := s.createPage(ctx, "about:blank")
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnavailable, "创建初始页面失败：%v", err)
	}
	return p, nil
}

// currentPage 返回当前页面。
func (s *Session) currentPage() (*pageState, *protocol.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, protocol.Errorf(protocol.CodeNotFound, "浏览器会话已关闭")
	}
	p := s.pages[s.current]
	if p == nil {
		return nil, protocol.Errorf(protocol.CodeInternal, "会话没有可用的页面")
	}
	return p, nil
}

// withPage 按 CDP sessionId 找到页面并执行 fn。
func (s *Session) withPage(sessionID string, fn func(p *pageState)) {
	s.mu.Lock()
	p := s.bySession[sessionID]
	s.mu.Unlock()
	if p != nil {
		fn(p)
	}
}

// close 关闭会话：先关连接，再结束进程，最后清理临时目录。
func (s *Session) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	client := s.client
	proc := s.proc
	s.mu.Unlock()
	if client != nil {
		client.close()
	}
	if proc != nil {
		proc.close()
	}
}

// cdpProtocolError 把 CDP 错误映射为协议错误。
func cdpProtocolError(err error) *protocol.Error {
	var ce *cdpError
	if errorsAsCDP(err, &ce) {
		switch {
		case ce.Code == -32000 && containsFold(ce.Message, "Cannot find context"):
			return protocol.Errorf(protocol.CodeInvalidArgs, "页面已导航或关闭：%s", ce.Message)
		case ce.Code == -32601:
			return protocol.Errorf(protocol.CodeUnsupportedPlatform, "浏览器不支持该操作：%s", ce.Message)
		}
		return protocol.Errorf(protocol.CodeInternal, "浏览器返回错误：%s", ce.Error())
	}
	return protocol.Errorf(protocol.CodeUnavailable, "浏览器连接异常：%v", err)
}
