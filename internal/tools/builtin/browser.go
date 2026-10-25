package builtin

// 浏览器工具：经 Chrome DevTools Protocol 操作被控端的浏览器。
// 参数和返回值的线上格式见 docs/tools.md 的“浏览器工具”一节。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Richard2091/DeskBridge/internal/browser"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// 浏览器管理器是进程内单例：节点和 ui-core 内置的节点共用数据目录和配置。
var (
	browserOnce sync.Once
	browserMgr  *browser.Manager
	browserErr  error
)

// browserManager 延迟创建浏览器管理器。配置和目录只在第一次真正用到
// 浏览器工具时读取，注册工具本身没有副作用。
func browserManager() (*browser.Manager, error) {
	browserOnce.Do(func() {
		home, err := paths.EnsureHome()
		if err != nil {
			browserErr = fmt.Errorf("准备数据目录失败：%w", err)
			return
		}
		browserMgr, browserErr = browser.NewManager(home, log.Printf)
	})
	return browserMgr, browserErr
}

// CloseBrowsers 关闭所有浏览器会话。节点退出时调用；进程结束时 Windows
// 的作业对象和 Linux 的 Pdeathsig 还会兜底结束浏览器进程。
func CloseBrowsers() {
	if browserMgr != nil {
		browserMgr.CloseAll()
	}
}

// browserOwner 返回会话归属键：同一种连接方式、同一个主人、同一台设备。
func browserOwner(call *tools.Call) string {
	id := call.Caller.Identity
	return string(id.Kind) + "|" + id.Owner + "|" + id.Device
}

// browserSession 取出调用者拥有的浏览器会话。
func browserSession(call *tools.Call, sessionID string) (*browser.Session, *protocol.Error) {
	if sessionID == "" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs,
			"session 不能为空；先调用 browser.open 获取会话 id")
	}
	mgr, err := browserManager()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnavailable, "%v", err)
	}
	return mgr.Get(browserOwner(call), sessionID)
}

// browserTools 返回全部浏览器工具。
func browserTools() []*tools.Tool {
	return []*tools.Tool{
		newBrowserOpenTool(),
		newBrowserCloseTool(),
		newBrowserInfoTool(),
		newBrowserNavigateTool(),
		newBrowserSnapshotTool(),
		newBrowserClickTool(),
		newBrowserTypeTool(),
		newBrowserKeyTool(),
		newBrowserWaitTool(),
		newBrowserScreenshotTool(),
		newBrowserEvalTool(),
		newBrowserConsoleTool(),
		newBrowserNetworkTool(),
		newBrowserTabsTool(),
		newBrowserCookiesTool(),
	}
}

// newBrowserTool 构造一个浏览器工具的公共部分。
func newBrowserTool(name, description, schema string, defaultTimeout time.Duration,
	run func(context.Context, *tools.Call) (any, error)) *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        name,
			Description: description,
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(schema),
		},
		DefaultTimeout: defaultTimeout,
		Run:            run,
	}
}

// ---------- browser.open ----------

const browserOpenSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "headless": {"type": "boolean", "description": "是否无头运行；不指定时用 browser.json 的配置（默认 true）"},
    "executable": {"type": "string", "description": "浏览器可执行文件路径；留空时使用系统默认浏览器"},
    "profile": {"type": "string", "enum": ["isolated", "persistent"], "description": "配置档案模式：isolated（默认，临时目录，关闭即删）或 persistent（保存在数据目录）"},
    "profile_name": {"type": "string", "description": "persistent 模式下区分不同配置档案的名字"},
    "viewport": {"type": "object", "additionalProperties": false, "description": "页面视口大小，默认 1280×800", "properties": {"width": {"type": "integer", "minimum": 200, "maximum": 3840}, "height": {"type": "integer", "minimum": 200, "maximum": 2160}}}
  }
}`

type browserOpenArgs struct {
	Headless    *bool             `json:"headless"`
	Executable  string            `json:"executable"`
	Profile     string            `json:"profile"`
	ProfileName string            `json:"profile_name"`
	Viewport    *browser.Viewport `json:"viewport"`
}

func newBrowserOpenTool() *tools.Tool {
	return newBrowserTool("browser.open",
		"打开一个浏览器会话（默认无头、使用系统默认浏览器），返回会话 id。之后的浏览器工具都要带这个 session。",
		browserOpenSchema, 30*time.Second, runBrowserOpen)
}

func runBrowserOpen(ctx context.Context, call *tools.Call) (any, error) {
	var a browserOpenArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	mgr, err := browserManager()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnavailable, "%v", err)
	}
	opts := browser.OpenOptions{
		Headless:    a.Headless,
		Executable:  a.Executable,
		Profile:     a.Profile,
		ProfileName: a.ProfileName,
	}
	if a.Viewport != nil {
		opts.Viewport = *a.Viewport
	}
	_, info, perr := mgr.Open(ctx, browserOwner(call), opts)
	if perr != nil {
		return nil, perr
	}
	return info, nil
}

// ---------- browser.close ----------

const browserCloseSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {"session": {"type": "string", "description": "browser.open 返回的会话 id"}}
}`

type browserCloseArgs struct {
	Session string `json:"session"`
}

func newBrowserCloseTool() *tools.Tool {
	return newBrowserTool("browser.close", "关闭浏览器会话，结束浏览器进程。", browserCloseSchema,
		30*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserCloseArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			if a.Session == "" {
				return nil, protocol.Errorf(protocol.CodeInvalidArgs, "session 不能为空")
			}
			mgr, err := browserManager()
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeUnavailable, "%v", err)
			}
			if perr := mgr.Close(browserOwner(call), a.Session); perr != nil {
				return nil, perr
			}
			return map[string]any{"closed": true, "session": a.Session}, nil
		})
}

// ---------- browser.info ----------

const browserInfoSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {}
}`

type browserInfoResult struct {
	ConfigPath string `json:"config_path"`
	Config     struct {
		Enabled            bool     `json:"enabled"`
		Headless           bool     `json:"headless"`
		MaxSessions        int      `json:"max_sessions"`
		IdleTimeoutSeconds int      `json:"idle_timeout_seconds"`
		AllowedSchemes     []string `json:"allowed_schemes"`
		ProfileMode        string   `json:"profile_mode"`
	} `json:"config"`
	Browsers browser.BrowserInfo   `json:"browsers"`
	Sessions []browser.SessionInfo `json:"sessions"`
}

func newBrowserInfoTool() *tools.Tool {
	return newBrowserTool("browser.info",
		"查看浏览器的可用情况：系统默认浏览器、可用的 Chromium 浏览器、当前配置和已打开的会话。不会启动浏览器。",
		browserInfoSchema, 10*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			mgr, err := browserManager()
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeUnavailable, "%v", err)
			}
			cfg := mgr.Config()
			out := browserInfoResult{
				ConfigPath: browser.ConfigPath(mgr.Home()),
				Browsers:   browser.DetectBrowsers(cfg),
				Sessions:   mgr.SessionInfos(),
			}
			out.Config.Enabled = cfg.Enabled
			out.Config.Headless = cfg.Headless
			out.Config.MaxSessions = cfg.MaxSessions
			out.Config.IdleTimeoutSeconds = cfg.IdleTimeoutSeconds
			out.Config.AllowedSchemes = cfg.AllowedSchemes
			out.Config.ProfileMode = cfg.ProfileMode
			return out, nil
		})
}

// ---------- browser.navigate ----------

const browserNavigateSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session", "url"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "url": {"type": "string", "description": "要打开的地址，必须带 scheme，如 https://example.com"},
    "wait_until": {"type": "string", "enum": ["load", "domcontentloaded", "network_idle"], "description": "等待页面就绪的条件，默认 load"}
  }
}`

type browserNavigateArgs struct {
	Session   string `json:"session"`
	URL       string `json:"url"`
	WaitUntil string `json:"wait_until"`
}

func newBrowserNavigateTool() *tools.Tool {
	return newBrowserTool("browser.navigate", "在当前标签页打开一个网址，等待页面加载后返回地址和标题。",
		browserNavigateSchema, 60*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserNavigateArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			res, err := s.Navigate(ctx, a.URL, a.WaitUntil)
			if err != nil {
				return nil, err
			}
			return res, nil
		})
}

// ---------- browser.snapshot ----------

const browserSnapshotSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "max_nodes": {"type": "integer", "minimum": 10, "maximum": 2000, "description": "最多返回多少个节点，默认 400"},
    "max_bytes": {"type": "integer", "minimum": 4096, "maximum": 1048576, "description": "结果的大致字节上限，默认 262144"}
  }
}`

type browserSnapshotArgs struct {
	Session  string `json:"session"`
	MaxNodes int    `json:"max_nodes"`
	MaxBytes int    `json:"max_bytes"`
}

func newBrowserSnapshotTool() *tools.Tool {
	return newBrowserTool("browser.snapshot",
		"读取当前页面的可访问性树，返回带 ref 编号的扁平节点列表（按钮、链接、输入框等）。点击和输入时用这里的 ref，比 CSS 选择器可靠。",
		browserSnapshotSchema, 60*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserSnapshotArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			return s.Snapshot(ctx, a.MaxNodes, a.MaxBytes)
		})
}

// ---------- browser.click ----------

const browserClickSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "ref": {"type": "string", "description": "browser.snapshot 返回的 ref，如 e12"},
    "selector": {"type": "string", "description": "CSS 选择器；和 ref、point 三选一"},
    "point": {"type": "object", "additionalProperties": false, "description": "页面坐标（CSS 像素）", "properties": {"x": {"type": "number"}, "y": {"type": "number"}}},
    "snapshot_id": {"type": "string", "description": "可选：ref 所属的快照 id，用于发现快照过期"},
    "button": {"type": "string", "enum": ["left", "right", "middle"], "description": "鼠标按键，默认 left"},
    "click_count": {"type": "integer", "minimum": 1, "maximum": 3, "description": "点击次数，默认 1"},
    "modifiers": {"type": "array", "items": {"type": "string", "enum": ["alt", "ctrl", "meta", "shift"]}, "description": "按住的修饰键"}
  }
}`

type browserClickArgs struct {
	Session    string          `json:"session"`
	Ref        string          `json:"ref"`
	Selector   string          `json:"selector"`
	Point      *browser.Point  `json:"point"`
	SnapshotID string          `json:"snapshot_id"`
	Button     string          `json:"button"`
	ClickCount int             `json:"click_count"`
	Modifiers  []string        `json:"modifiers"`
}

func newBrowserClickTool() *tools.Tool {
	return newBrowserTool("browser.click",
		"点击页面元素：按 ref、CSS 选择器或坐标。点击前会滚动到可见位置。",
		browserClickSchema, 30*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserClickArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			if err := s.Click(ctx, browser.ClickOptions{
				Ref:        a.Ref,
				Selector:   a.Selector,
				Point:      a.Point,
				Button:     a.Button,
				ClickCount: a.ClickCount,
				Modifiers:  a.Modifiers,
			}); err != nil {
				return nil, err
			}
			url, title := s.PageInfo()
			return map[string]any{"clicked": true, "url": url, "title": title}, nil
		})
}

// ---------- browser.type ----------

const browserTypeSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session", "text"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "ref": {"type": "string", "description": "browser.snapshot 返回的 ref"},
    "selector": {"type": "string", "description": "CSS 选择器；和 ref 二选一"},
    "text": {"type": "string", "description": "要输入的文字，支持中文和 emoji"},
    "clear": {"type": "boolean", "description": "先清空原有内容"},
    "submit": {"type": "boolean", "description": "输入后按一次 Enter"}
  }
}`

type browserTypeArgs struct {
	Session  string `json:"session"`
	Ref      string `json:"ref"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Clear    bool   `json:"clear"`
	Submit   bool   `json:"submit"`
}

func newBrowserTypeTool() *tools.Tool {
	return newBrowserTool("browser.type", "在输入框里输入文字（先聚焦元素），可选清空和回车提交。",
		browserTypeSchema, 30*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserTypeArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			n, err := s.Type(ctx, a.Ref, a.Selector, a.Text, a.Clear, a.Submit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"typed": n}, nil
		})
}

// ---------- browser.key ----------

const browserKeySchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session", "key"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "key": {"type": "string", "description": "按键，如 Enter、Tab、Escape、ArrowDown、a"},
    "modifiers": {"type": "array", "items": {"type": "string", "enum": ["alt", "ctrl", "meta", "shift"]}, "description": "按住的修饰键"}
  }
}`

type browserKeyArgs struct {
	Session   string   `json:"session"`
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers"`
}

func newBrowserKeyTool() *tools.Tool {
	return newBrowserTool("browser.key", "按一次键，可带 Ctrl、Shift 等修饰键，例如 Enter、Tab、ArrowDown、Ctrl+A。",
		browserKeySchema, 20*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserKeyArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			if err := s.Key(ctx, a.Key, a.Modifiers); err != nil {
				return nil, err
			}
			return map[string]any{"pressed": a.Key}, nil
		})
}

// ---------- browser.wait ----------

const browserWaitSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "selector": {"type": "string", "description": "等待这个 CSS 选择器"},
    "text": {"type": "string", "description": "等待页面文本包含这个内容"},
    "url": {"type": "string", "description": "等待地址包含这个内容"},
    "state": {"type": "string", "enum": ["visible", "attached"], "description": "选择器的状态，默认 visible"}
  }
}`

type browserWaitArgs struct {
	Session  string `json:"session"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
	URL      string `json:"url"`
	State    string `json:"state"`
}

func newBrowserWaitTool() *tools.Tool {
	return newBrowserTool("browser.wait", "等待页面满足条件：元素出现、文本出现或地址变化。",
		browserWaitSchema, 60*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserWaitArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			ms, err := s.Wait(ctx, a.Selector, a.Text, a.URL, a.State)
			if err != nil {
				return nil, err
			}
			return map[string]any{"waited_ms": ms}, nil
		})
}

// ---------- browser.screenshot ----------

const browserScreenshotSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "full_page": {"type": "boolean", "description": "截取整个页面，默认只截视口"},
    "format": {"type": "string", "enum": ["png", "jpeg"], "description": "图片格式，默认 png"},
    "quality": {"type": "integer", "minimum": 1, "maximum": 100, "description": "jpeg 质量"},
    "clip": {"type": "object", "additionalProperties": false, "description": "只截取这个区域（CSS 像素）", "properties": {"x": {"type": "number"}, "y": {"type": "number"}, "width": {"type": "number"}, "height": {"type": "number"}}},
    "save_path": {"type": "string", "description": "保存路径；相对路径相对数据目录，留空存到数据目录 browser/screenshots"}
  }
}`

type browserScreenshotArgs struct {
	Session  string        `json:"session"`
	FullPage bool          `json:"full_page"`
	Format   string        `json:"format"`
	Quality  int           `json:"quality"`
	Clip     *browser.Clip `json:"clip"`
	SavePath string        `json:"save_path"`
}

func newBrowserScreenshotTool() *tools.Tool {
	return newBrowserTool("browser.screenshot", "截取页面并保存为文件，返回文件路径和尺寸。",
		browserScreenshotSchema, 60*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserScreenshotArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			return s.Screenshot(ctx, a.FullPage, a.Format, a.Quality, a.Clip, a.SavePath)
		})
}

// ---------- browser.eval ----------

const browserEvalSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session", "expression"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "expression": {"type": "string", "description": "在页面里执行的 JavaScript 表达式，返回值按 JSON 返回"},
    "await_promise": {"type": "boolean", "description": "表达式返回 Promise 时等待它完成"},
    "max_bytes": {"type": "integer", "minimum": 1024, "maximum": 1048576, "description": "返回值上限，默认 262144，超出时返回 preview"}
  }
}`

type browserEvalArgs struct {
	Session      string `json:"session"`
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise"`
	MaxBytes     int    `json:"max_bytes"`
}

func newBrowserEvalTool() *tools.Tool {
	return newBrowserTool("browser.eval", "在当前页面执行 JavaScript 并返回结果（等同于在该页面上运行脚本，权限较高）。",
		browserEvalSchema, 60*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserEvalArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			maxBytes := a.MaxBytes
			if maxBytes <= 0 {
				maxBytes = 256 << 10
			}
			if maxBytes > 1<<20 {
				maxBytes = 1 << 20
			}
			value, truncated, err := s.Eval(ctx, a.Expression, a.AwaitPromise, maxBytes)
			if err != nil {
				return nil, err
			}
			return map[string]any{"value": value, "truncated": truncated}, nil
		})
}

// ---------- browser.console ----------

const browserConsoleSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "level": {"type": "string", "description": "最低级别：debug、log、info、warning、error；留空返回全部"},
    "max_entries": {"type": "integer", "minimum": 1, "maximum": 1000, "description": "最多返回多少条，默认 200"},
    "clear": {"type": "boolean", "description": "读取后清空缓冲"}
  }
}`

type browserConsoleArgs struct {
	Session    string `json:"session"`
	Level      string `json:"level"`
	MaxEntries int    `json:"max_entries"`
	Clear      bool   `json:"clear"`
}

func newBrowserConsoleTool() *tools.Tool {
	return newBrowserTool("browser.console", "查看页面的控制台消息和未捕获异常。",
		browserConsoleSchema, 20*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserConsoleArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			maxEntries := a.MaxEntries
			if maxEntries <= 0 {
				maxEntries = 200
			}
			entries, err := s.Console(ctx, a.Level, maxEntries, a.Clear)
			if err != nil {
				return nil, err
			}
			return map[string]any{"entries": entries, "count": len(entries)}, nil
		})
}

// ---------- browser.network ----------

const browserNetworkSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "filter": {"type": "string", "enum": ["all", "xhr", "document", "failed"], "description": "只看某类请求，默认 all"},
    "max_entries": {"type": "integer", "minimum": 1, "maximum": 1000, "description": "最多返回多少条，默认 200"}
  }
}`

type browserNetworkArgs struct {
	Session    string `json:"session"`
	Filter     string `json:"filter"`
	MaxEntries int    `json:"max_entries"`
}

func newBrowserNetworkTool() *tools.Tool {
	return newBrowserTool("browser.network", "查看页面发出的网络请求（方法、地址、状态码、耗时、是否失败）。",
		browserNetworkSchema, 20*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserNetworkArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			maxEntries := a.MaxEntries
			if maxEntries <= 0 {
				maxEntries = 200
			}
			entries, err := s.Network(ctx, a.Filter, maxEntries)
			if err != nil {
				return nil, err
			}
			return map[string]any{"requests": entries, "count": len(entries)}, nil
		})
}

// ---------- browser.tabs ----------

const browserTabsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "action": {"type": "string", "enum": ["list", "new", "select", "close"], "description": "操作，默认 list"},
    "url": {"type": "string", "description": "action=new 时打开的地址，默认 about:blank"},
    "index": {"type": "integer", "minimum": 0, "description": "action=select/close 时的标签页序号（从 0 开始）"}
  }
}`

type browserTabsArgs struct {
	Session string `json:"session"`
	Action  string `json:"action"`
	URL     string `json:"url"`
	Index   int    `json:"index"`
}

func newBrowserTabsTool() *tools.Tool {
	return newBrowserTool("browser.tabs", "列出、新建、切换或关闭标签页。",
		browserTabsSchema, 20*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserTabsArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			action := a.Action
			if action == "" {
				action = "list"
			}
			switch action {
			case "list":
				tabs, err := s.Tabs(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"tabs": tabs}, nil
			case "new":
				tabs, err := s.NewTab(ctx, a.URL)
				if err != nil {
					return nil, err
				}
				return map[string]any{"tabs": tabs}, nil
			case "select", "close":
				if a.Index < 0 {
					return nil, protocol.Errorf(protocol.CodeInvalidArgs, "action=%s 需要 index", action)
				}
				var (
					tabs []browser.Tab
					err  error
				)
				if action == "select" {
					tabs, err = s.SelectTab(ctx, a.Index)
				} else {
					tabs, err = s.CloseTab(ctx, a.Index)
				}
				if err != nil {
					return nil, err
				}
				return map[string]any{"tabs": tabs}, nil
			default:
				return nil, protocol.Errorf(protocol.CodeInvalidArgs,
					"action 只能是 list、new、select 或 close")
			}
		})
}

// ---------- browser.cookies ----------

const browserCookiesSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["session"],
  "properties": {
    "session": {"type": "string", "description": "会话 id"},
    "action": {"type": "string", "enum": ["get", "set", "clear"], "description": "操作，默认 get"},
    "url": {"type": "string", "description": "action=set 时 Cookie 所属的地址"},
    "cookies": {"type": "array", "description": "action=set 时要设置的 Cookie", "items": {"type": "object", "additionalProperties": false, "required": ["name", "value"], "properties": {"name": {"type": "string"}, "value": {"type": "string"}, "domain": {"type": "string"}, "path": {"type": "string"}, "expires": {"type": "number"}, "http_only": {"type": "boolean"}, "secure": {"type": "boolean"}, "same_site": {"type": "string"}}}}
  }
}`

type browserCookiesArgs struct {
	Session string           `json:"session"`
	Action  string           `json:"action"`
	URL     string           `json:"url"`
	Cookies []browser.Cookie `json:"cookies"`
}

func newBrowserCookiesTool() *tools.Tool {
	return newBrowserTool("browser.cookies", "读取、设置或清除浏览器的 Cookie。Cookie 的值在操作记录里会脱敏。",
		browserCookiesSchema, 20*time.Second, func(ctx context.Context, call *tools.Call) (any, error) {
			var a browserCookiesArgs
			if err := tools.DecodeArgs(call.Args, &a); err != nil {
				return nil, err
			}
			s, perr := browserSession(call, a.Session)
			if perr != nil {
				return nil, perr
			}
			action := a.Action
			if action == "" {
				action = "get"
			}
			switch action {
			case "get":
				cookies, err := s.Cookies(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"cookies": cookies, "count": len(cookies)}, nil
			case "set":
				if len(a.Cookies) == 0 {
					return nil, protocol.Errorf(protocol.CodeInvalidArgs, "action=set 需要 cookies")
				}
				if err := s.SetCookies(ctx, a.URL, a.Cookies); err != nil {
					return nil, err
				}
				return map[string]any{"set": len(a.Cookies)}, nil
			case "clear":
				if err := s.ClearCookies(ctx); err != nil {
					return nil, err
				}
				return map[string]any{"cleared": true}, nil
			default:
				return nil, protocol.Errorf(protocol.CodeInvalidArgs, "action 只能是 get、set 或 clear")
			}
		})
}
