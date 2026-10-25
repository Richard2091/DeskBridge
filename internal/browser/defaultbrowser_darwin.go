//go:build darwin

package browser

// macOS：默认浏览器记录在 LaunchServices 的偏好设置里，没有稳定的命令行
// 接口。先读 com.apple.launchservices.secure 的 LSHandlers，再按 bundle ID
// 找应用可执行文件；读不到时退回常见安装位置。macOS 支持尚未在真机验证。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultBrowser 返回系统默认浏览器。
func defaultBrowser() (Candidate, bool) {
	if id := defaultHTTPHandlerBundleID(); id != "" {
		if path := executableForBundleID(id); path != "" {
			return Candidate{
				Name:    browserName(path),
				Path:    path,
				Method:  "launchservices",
				Default: true,
				CDP:     chromiumByPath(path),
			}, true
		}
	}
	return Candidate{}, false
}

// defaultHTTPHandlerBundleID 从 LaunchServices 的 LSHandlers 中找出 http 的默认处理器。
func defaultHTTPHandlerBundleID() string {
	out, err := exec.Command("defaults", "read",
		"com.apple.LaunchServices/com.apple.launchservices.secure", "LSHandlers").Output()
	if err != nil {
		return ""
	}
	text := string(out)
	for _, block := range strings.Split(text, "}") {
		if !strings.Contains(block, "LSHandlerURLScheme = http") &&
			!strings.Contains(block, "LSHandlerURLScheme = \"http\"") {
			continue
		}
		if id := openStepValue(block, "LSHandlerRoleAll"); id != "" {
			return id
		}
	}
	return ""
}

// openStepValue 从 OpenStep 格式的片段中取出最简单的字符串值。
func openStepValue(block, key string) string {
	idx := strings.Index(block, key)
	if idx < 0 {
		return ""
	}
	rest := block[idx+len(key):]
	if eq := strings.Index(rest, "="); eq >= 0 {
		rest = rest[eq+1:]
	}
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, ";,")
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, `"`) {
		if end := strings.Index(rest[1:], `"`); end >= 0 {
			return rest[1 : 1+end]
		}
		return ""
	}
	if end := strings.IndexAny(rest, ";, \n"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return rest
}

// executableForBundleID 按 bundle ID 找到应用内的可执行文件。
func executableForBundleID(bundleID string) string {
	known := map[string]string{
		"com.google.chrome":       "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"com.google.chrome.beta":  "/Applications/Google Chrome Beta.app/Contents/MacOS/Google Chrome Beta",
		"com.microsoft.edgemac":   "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"org.chromium.chromium":   "/Applications/Chromium.app/Contents/MacOS/Chromium",
		"com.brave.browser":       "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"com.vivaldi.vivaldi":     "/Applications/Vivaldi.app/Contents/MacOS/Vivaldi",
		"com.operasoftware.opera": "/Applications/Opera.app/Contents/MacOS/Opera",
		"com.apple.safari":        "/Applications/Safari.app/Contents/MacOS/Safari",
	}
	if path, ok := known[strings.ToLower(bundleID)]; ok {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
		return ""
	}
	// 未知的浏览器：用 Spotlight 找到应用包，再读 Info.plist 里的可执行文件名。
	out, err := exec.Command("mdfind", "kMDItemCFBundleIdentifier == '"+bundleID+"'").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return ""
	}
	app := lines[0]
	if !strings.HasSuffix(app, ".app") || strings.Contains(app, "\n") {
		return ""
	}
	exe, err := exec.Command("defaults", "read",
		filepath.Join(app, "Contents", "Info"), "CFBundleExecutable").Output()
	if err != nil {
		return ""
	}
	return filepath.Join(app, "Contents", "MacOS", strings.TrimSpace(string(exe)))
}

// fallbackCandidates 返回常见安装位置里的 Chromium 浏览器。
func fallbackCandidates() []Candidate {
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	}
	var out []Candidate
	for _, path := range paths {
		out = appendCandidate(out, Candidate{
			Name:   browserName(path),
			Path:   path,
			Method: "fallback",
			CDP:    chromiumByPath(path),
		})
	}
	return out
}
