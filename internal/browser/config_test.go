package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	home := t.TempDir()
	cfg, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Enabled || !cfg.Headless {
		t.Errorf("默认应启用且无头：%+v", cfg)
	}
	if cfg.MaxSessions != DefaultMaxSessions {
		t.Errorf("max_sessions = %d，期望 %d", cfg.MaxSessions, DefaultMaxSessions)
	}
	if cfg.IdleTimeoutSeconds != int(DefaultIdleTimeout/time.Second) {
		t.Errorf("idle_timeout_seconds = %d", cfg.IdleTimeoutSeconds)
	}
	if !strings.Contains(strings.Join(cfg.AllowedSchemes, ","), "https") {
		t.Errorf("默认 scheme 应包含 https：%v", cfg.AllowedSchemes)
	}
	if cfg.schemeAllowed("file") {
		t.Error("默认不应允许 file scheme")
	}
	if !cfg.useDefaultBrowser() || !cfg.allowFallback() {
		t.Error("默认应使用系统默认浏览器并允许回退")
	}
}

func TestLoadConfigFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ConfigFileName)
	content := `{"headless": false, "max_sessions": 9, "idle_timeout_seconds": 99999, "allowed_schemes": ["HTTPS", " file ", ""], "profile_mode": "bogus"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Headless {
		t.Error("headless 应为 false")
	}
	if cfg.MaxSessions != MaxMaxSessions {
		t.Errorf("max_sessions 应被截到 %d，实际 %d", MaxMaxSessions, cfg.MaxSessions)
	}
	if time.Duration(cfg.IdleTimeoutSeconds)*time.Second != MaxIdleTimeout {
		t.Errorf("idle_timeout_seconds 应被截到 %s，实际 %d", MaxIdleTimeout, cfg.IdleTimeoutSeconds)
	}
	if len(cfg.AllowedSchemes) != 2 || cfg.AllowedSchemes[0] != "https" || cfg.AllowedSchemes[1] != "file" {
		t.Errorf("scheme 应去掉空项并转小写：%v", cfg.AllowedSchemes)
	}
	if cfg.ProfileMode != DefaultProfileMode {
		t.Errorf("非法 profile_mode 应回落到 %s，实际 %s", DefaultProfileMode, cfg.ProfileMode)
	}
}

func TestLoadConfigBroken(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ConfigFileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(home); err == nil {
		t.Fatal("损坏的配置应报错")
	}
}

func TestProfileDir(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.profileDir("/home/u", "abc", ""); !strings.HasSuffix(got, filepath.Join("profiles", "session-abc")) {
		t.Errorf("isolated profile 路径不对：%s", got)
	}
	cfg.ProfileMode = "persistent"
	if got := cfg.profileDir("/home/u", "abc", "../坏 名字"); !strings.HasSuffix(got, filepath.Join("profiles", "___")) {
		t.Errorf("persistent profile 名字应被清洗：%s", got)
	}
	if got := cfg.profileDir("/home/u", "abc", ""); !strings.HasSuffix(got, filepath.Join("profiles", "default")) {
		t.Errorf("persistent 默认名字不对：%s", got)
	}
}

func TestChromiumDetection(t *testing.T) {
	chrome := filepath.Join(t.TempDir(), "chrome.exe")
	edge := filepath.Join(t.TempDir(), "msedge.exe")
	cent := filepath.Join(t.TempDir(), "centbrowser.exe")
	firefox := filepath.Join(t.TempDir(), "firefox.exe")
	cases := []struct {
		path string
		want bool
	}{
		{chrome, true},
		{edge, true},
		{cent, true},
		{firefox, false},
		{filepath.Join(t.TempDir(), "notepad"), false},
	}
	for _, c := range cases {
		if got := chromiumByPath(c.path); got != c.want {
			t.Errorf("chromiumByPath(%q) = %v，期望 %v", c.path, got, c.want)
		}
	}
	if name := browserName(chrome); name != "Chrome" {
		t.Errorf("browserName = %q", name)
	}
	if name := browserName(edge); name != "Edge" {
		t.Errorf("browserName = %q", name)
	}
}

func TestExpandUserPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("没有用户目录")
	}
	if got := expandUserPath("~/bin/chrome"); got != filepath.Join(home, "bin", "chrome") {
		t.Errorf("expandUserPath = %q", got)
	}
	if got := expandUserPath("/opt/chrome"); got != "/opt/chrome" {
		t.Errorf("绝对路径不应改变：%q", got)
	}
}
