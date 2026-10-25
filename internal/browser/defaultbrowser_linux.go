//go:build linux

package browser

// Linux：用 xdg-settings / xdg-mime 读取默认浏览器，再解析 .desktop 文件里的
// Exec 命令行。没有桌面环境的服务器上通常两者都没有，退回 PATH 里的
// Chromium 浏览器。

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultBrowser 返回系统默认浏览器。
func defaultBrowser() (Candidate, bool) {
	if out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output(); err == nil {
		desktop := strings.TrimSpace(string(out))
		if path := desktopExecutable(desktop); path != "" {
			return Candidate{
				Name:    browserName(path),
				Path:    path,
				Method:  "xdg-settings",
				Default: true,
				CDP:     chromiumByPath(path),
			}, true
		}
	}
	if out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/http").Output(); err == nil {
		desktop := strings.TrimSpace(string(out))
		if path := desktopExecutable(desktop); path != "" {
			return Candidate{
				Name:    browserName(path),
				Path:    path,
				Method:  "xdg-mime",
				Default: true,
				CDP:     chromiumByPath(path),
			}, true
		}
	}
	return Candidate{}, false
}

// desktopExecutable 从 .desktop 文件名解析出可执行文件路径。
func desktopExecutable(desktop string) string {
	if desktop == "" {
		return ""
	}
	name := desktop
	if !strings.HasSuffix(name, ".desktop") {
		name += ".desktop"
	}
	for _, dir := range desktopDirs() {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if exec := execFromDesktopFile(path); exec != "" {
				return exec
			}
		}
	}
	return ""
}

// desktopDirs 返回 .desktop 文件的搜索目录。
func desktopDirs() []string {
	var dirs []string
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		dirs = append(dirs, filepath.Join(v, "applications"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "applications"))
	}
	if v := os.Getenv("XDG_DATA_DIRS"); v != "" {
		for _, dir := range strings.Split(v, string(os.PathListSeparator)) {
			if dir != "" {
				dirs = append(dirs, filepath.Join(dir, "applications"))
			}
		}
	}
	dirs = append(dirs, "/usr/share/applications", "/usr/local/share/applications")
	return dirs
}

// execFromDesktopFile 读取 .desktop 文件的 Exec= 行，去掉 %U 之类的占位符，
// 返回可执行文件路径（相对路径按 PATH 查找）。
func execFromDesktopFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Exec=") {
			continue
		}
		cmdline := strings.TrimPrefix(line, "Exec=")
		fields := strings.Fields(cmdline)
		if len(fields) == 0 {
			return ""
		}
		exe := strings.Trim(fields[0], `"'`)
		exe = strings.TrimSuffix(exe, "%U")
		exe = strings.TrimSuffix(exe, "%F")
		if exe == "" {
			return ""
		}
		if !filepath.IsAbs(exe) {
			if found, err := exec.LookPath(exe); err == nil {
				return found
			}
			continue
		}
		if info, err := os.Stat(exe); err == nil && !info.IsDir() {
			return exe
		}
	}
	return ""
}

// fallbackCandidates 返回 PATH 里已知的 Chromium 浏览器。
func fallbackCandidates() []Candidate {
	names := []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"microsoft-edge", "microsoft-edge-stable", "brave-browser", "vivaldi",
	}
	var out []Candidate
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		out = appendCandidate(out, Candidate{
			Name:   browserName(path),
			Path:   path,
			Method: "fallback",
			CDP:    chromiumByPath(path),
		})
	}
	return out
}
