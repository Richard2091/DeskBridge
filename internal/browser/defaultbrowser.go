package browser

// 浏览器选择：优先系统设置的默认浏览器，其次配置指定的可执行文件，
// 最后退回已知的 Chromium 内核浏览器。只有 Chromium 内核支持 CDP，
// 所以默认浏览器是 Firefox、Safari 等时无法自动化，会给出提示并（在允许时）
// 退回 Chromium。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Candidate 是一个可启动的浏览器。
type Candidate struct {
	Name    string `json:"name"`    // 给人看的名字，如 Chrome、Edge
	Path    string `json:"path"`    // 可执行文件路径
	Method  string `json:"method"`  // 来源：windows-userchoice、xdg-settings、launchservices、config、fallback
	Default bool   `json:"default"` // 是否是系统默认浏览器
	CDP     bool   `json:"cdp"`     // 是否支持 Chrome DevTools Protocol
}

// 选择浏览器时的错误。
var (
	// ErrNoBrowser 表示这台设备上找不到可用的 Chromium 内核浏览器。
	ErrNoBrowser = errors.New("没有找到支持 CDP 的浏览器（需要 Chrome、Edge、Chromium 等）")
	// ErrDefaultBrowserUnsupported 表示默认浏览器不支持 CDP，且不允许退回。
	ErrDefaultBrowserUnsupported = errors.New("系统默认浏览器不支持 CDP")
)

// chromiumByPath 按可执行文件名判断是否是 Chromium 内核的浏览器。
// Cent Browser、Brave 等第三方 Chromium 浏览器同样支持 CDP。
func chromiumByPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "chrome", "chrome_proxy", "chromium", "chromium-browser", "google-chrome",
		"google-chrome-stable", "msedge", "microsoft-edge", "microsoft-edge-stable",
		"centbrowser", "brave", "brave-browser", "vivaldi", "opera", "thorium",
		"arc", "yandex", "yandexbrowser", "360chrome", "360se", "qqbrowser",
		"chromedriver":
		return true
	}
	// Chrome 的测试版、开发版等以 chrome- 开头。
	if strings.HasPrefix(base, "chrome-") {
		return true
	}
	return false
}

// browserName 根据可执行文件名给出浏览器名字。
func browserName(path string) string {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ".exe"))
	switch {
	case base == "chrome" || strings.HasPrefix(base, "chrome-"):
		return "Chrome"
	case base == "msedge" || strings.HasPrefix(base, "msedge"):
		return "Edge"
	case strings.Contains(base, "chromium"):
		return "Chromium"
	case strings.Contains(base, "centbrowser"):
		return "Cent Browser"
	case strings.Contains(base, "brave"):
		return "Brave"
	case strings.Contains(base, "vivaldi"):
		return "Vivaldi"
	case strings.Contains(base, "opera"):
		return "Opera"
	case strings.Contains(base, "thorium"):
		return "Thorium"
	case strings.Contains(base, "arc"):
		return "Arc"
	default:
		if base == "" {
			return "浏览器"
		}
		return base
	}
}

// candidates 返回按优先级排列的可启动浏览器列表，以及需要告诉用户的提示。
//
// 顺序：配置指定的可执行文件 → 系统默认浏览器（支持 CDP 时）→ 已知的
// Chromium 浏览器。默认浏览器不支持 CDP 时是否允许退回由配置决定。
func candidates(cfg Config, configured string) ([]Candidate, []string, error) {
	var out []Candidate
	var warnings []string

	if configured != "" {
		path := expandUserPath(configured)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return nil, warnings, fmt.Errorf("配置的浏览器 %s 不存在", configured)
		}
		out = append(out, Candidate{
			Name:   browserName(path),
			Path:   path,
			Method: "config",
			CDP:    true, // 用户显式指定，按支持 CDP 处理
		})
	}

	if cfg.useDefaultBrowser() {
		if c, ok := defaultBrowser(); ok {
			if c.CDP {
				out = appendCandidate(out, c)
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"系统默认浏览器 %s（%s）不是 Chromium 内核，不支持 CDP", c.Name, c.Path))
				if !cfg.allowFallback() {
					return nil, warnings, ErrDefaultBrowserUnsupported
				}
			}
		}
	}

	if cfg.allowFallback() {
		for _, c := range fallbackCandidates() {
			out = appendCandidate(out, c)
		}
	}

	if len(out) == 0 {
		if len(warnings) > 0 {
			return nil, warnings, fmt.Errorf("%w：%s", ErrNoBrowser, strings.Join(warnings, "；"))
		}
		return nil, warnings, ErrNoBrowser
	}
	return out, warnings, nil
}

// appendCandidate 追加一个浏览器，跳过路径重复或文件不存在的项。
func appendCandidate(list []Candidate, c Candidate) []Candidate {
	if c.Path == "" {
		return list
	}
	info, err := os.Stat(c.Path)
	if err != nil || info.IsDir() {
		return list
	}
	for _, existing := range list {
		if samePath(existing.Path, c.Path) {
			return list
		}
	}
	return append(list, c)
}

// samePath 比较两个路径是否指向同一个文件（大小写不敏感，Windows 需要）。
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// expandUserPath 展开以 ~ 开头的路径，方便配置里写 ~/bin/chrome。
func expandUserPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), string(filepath.Separator)))
		}
	}
	return path
}

// BrowserInfo 是 browser.info 返回的系统浏览器信息。
type BrowserInfo struct {
	Default    Candidate   `json:"default"`
	HasDefault bool        `json:"has_default"`
	Candidates []Candidate `json:"candidates"`
	Warnings   []string    `json:"warnings,omitempty"`
}

// DetectBrowsers 返回当前设备的默认浏览器和可用的 Chromium 浏览器，
// 供 browser.info 展示；不会启动浏览器。
func DetectBrowsers(cfg Config) BrowserInfo {
	info := BrowserInfo{}
	if c, ok := defaultBrowser(); ok {
		info.Default = c
		info.HasDefault = true
	}
	list, warnings, err := candidates(cfg, cfg.Executable)
	if err == nil {
		info.Candidates = list
	}
	info.Warnings = append(info.Warnings, warnings...)
	if err != nil && len(info.Warnings) == 0 {
		info.Warnings = append(info.Warnings, err.Error())
	}
	return info
}
