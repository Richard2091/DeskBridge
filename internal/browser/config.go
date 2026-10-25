// Package browser 通过 Chrome DevTools Protocol（CDP）驱动被控端的浏览器。
//
// 节点不内置浏览器引擎：它启动系统默认（或配置指定）的 Chromium 内核浏览器，
// 使用独立的用户数据目录，经 CDP 控制，默认无头运行。设计见
// docs/decisions/adr-0012-browser-tools-cdp.md，工具参数和返回值的线上格式
// 见 docs/tools.md。
package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 配置文件和会话的默认值。
const (
	// ConfigFileName 是数据目录下的浏览器设置文件。
	ConfigFileName = "browser.json"
	// DefaultMaxSessions 是同时打开的浏览器会话上限。
	DefaultMaxSessions = 2
	// MaxMaxSessions 是 max_sessions 的上限，避免一台设备被开满浏览器。
	MaxMaxSessions = 4
	// DefaultIdleTimeout 是会话空闲多久后自动关闭。
	DefaultIdleTimeout = 10 * time.Minute
	// MaxIdleTimeout 是 idle_timeout_seconds 的上限。
	MaxIdleTimeout = time.Hour
	// DefaultProfileMode 是默认的配置档案模式：每个会话一个临时目录。
	DefaultProfileMode = "isolated"
)

// Config 是 browser.json 的结构。字段都可缺省，零值表示未设置，
// 由 withDefaults 套用默认值。
type Config struct {
	// Enabled 为 false 时不启动浏览器，浏览器工具返回 unavailable。
	Enabled bool `json:"enabled"`
	// Headless 是默认是否无头运行（默认 true）。
	Headless bool `json:"headless"`
	// Executable 指定浏览器可执行文件；留空时用系统默认浏览器。
	Executable string `json:"executable"`
	// UseDefaultBrowser 为 false 时不使用系统默认浏览器，只用配置或已知的
	// Chromium 浏览器（默认 true）。
	UseDefaultBrowser *bool `json:"use_default_browser"`
	// AllowFallback 为 true 时，默认浏览器不支持 CDP 或启动失败会退回已知的
	// Chromium 浏览器（默认 true）。
	AllowFallback *bool `json:"allow_fallback"`
	// MaxSessions 是同时打开的会话上限（默认 2，上限 4）。
	MaxSessions int `json:"max_sessions"`
	// IdleTimeoutSeconds 是空闲回收时间（默认 600 秒，上限 3600）。
	IdleTimeoutSeconds int `json:"idle_timeout_seconds"`
	// AllowedSchemes 是允许导航的 URL scheme（默认 http、https、about、data；
	// 不包含 file）。
	AllowedSchemes []string `json:"allowed_schemes"`
	// ProfileMode 是 isolated（默认，每会话临时目录）或 persistent。
	ProfileMode string `json:"profile_mode"`
}

// DefaultConfig 返回全部取默认值的配置。
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		Headless:           true,
		MaxSessions:        DefaultMaxSessions,
		IdleTimeoutSeconds: int(DefaultIdleTimeout / time.Second),
		AllowedSchemes:     []string{"http", "https", "about", "data"},
		ProfileMode:        DefaultProfileMode,
	}
}

// ConfigPath 返回数据目录下的 browser.json 路径。
func ConfigPath(home string) string {
	return filepath.Join(home, ConfigFileName)
}

// LoadConfig 读取数据目录下的 browser.json。文件不存在时返回默认配置，
// 不再报错；文件损坏时返回错误并说明路径。
func LoadConfig(home string) (Config, error) {
	cfg := DefaultConfig()
	if home == "" {
		return cfg, nil
	}
	path := ConfigPath(home)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("读取浏览器设置 %s 失败：%w", path, err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("浏览器设置 %s 不是有效的 JSON：%w", path, err)
	}
	return cfg.withDefaults(), nil
}

// withDefaults 填补零值并检查取值范围。
func (c Config) withDefaults() Config {
	if c.MaxSessions <= 0 {
		c.MaxSessions = DefaultMaxSessions
	}
	if c.MaxSessions > MaxMaxSessions {
		c.MaxSessions = MaxMaxSessions
	}
	if c.IdleTimeoutSeconds <= 0 {
		c.IdleTimeoutSeconds = int(DefaultIdleTimeout / time.Second)
	}
	if time.Duration(c.IdleTimeoutSeconds)*time.Second > MaxIdleTimeout {
		c.IdleTimeoutSeconds = int(MaxIdleTimeout / time.Second)
	}
	if len(c.AllowedSchemes) == 0 {
		c.AllowedSchemes = DefaultConfig().AllowedSchemes
	}
	schemes := make([]string, 0, len(c.AllowedSchemes))
	for _, s := range c.AllowedSchemes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			schemes = append(schemes, s)
		}
	}
	c.AllowedSchemes = schemes
	switch c.ProfileMode {
	case "", "isolated":
		c.ProfileMode = DefaultProfileMode
	case "persistent":
	default:
		c.ProfileMode = DefaultProfileMode
	}
	return c
}

// useDefaultBrowser 返回是否使用系统默认浏览器；未设置时为 true。
func (c Config) useDefaultBrowser() bool {
	return c.UseDefaultBrowser == nil || *c.UseDefaultBrowser
}

// allowFallback 返回默认浏览器不可用时是否退回其他 Chromium 浏览器。
func (c Config) allowFallback() bool {
	return c.AllowFallback == nil || *c.AllowFallback
}

// idleTimeout 返回会话空闲回收时间。
func (c Config) idleTimeout() time.Duration {
	return time.Duration(c.IdleTimeoutSeconds) * time.Second
}

// schemeAllowed 判断 URL scheme 是否允许。
func (c Config) schemeAllowed(scheme string) bool {
	scheme = strings.ToLower(scheme)
	for _, s := range c.AllowedSchemes {
		if s == scheme {
			return true
		}
	}
	return false
}

// profileDir 返回一个配置档案目录。isolated 时为每次会话的临时目录，
// persistent 时为数据目录下按名字区分的目录。
func (c Config) profileDir(home, sessionID, name string) string {
	root := filepath.Join(home, "browser", "profiles")
	if c.ProfileMode == "persistent" {
		if name == "" {
			name = "default"
		}
		return filepath.Join(root, sanitizeName(name))
	}
	return filepath.Join(root, "session-"+sessionID)
}

// screenshotDir 返回截图默认保存目录。
func (c Config) screenshotDir(home string) string {
	return filepath.Join(home, "browser", "screenshots")
}

// sanitizeName 把用户提供的名字限制为安全的文件名字符。
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "default"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
