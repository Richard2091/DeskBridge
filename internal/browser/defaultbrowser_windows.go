//go:build windows

package browser

// Windows：从注册表读取默认浏览器。Windows 10 起默认程序由
// HKCU\...\UrlAssociations\http\UserChoice 的 ProgId 决定，ProgId 对应的
// 命令行在 HKCR\<ProgId>\shell\open\command。

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// userChoiceProgID 返回当前用户对 http 协议的默认程序 ProgId。
func userChoiceProgID() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// progIDCommand 返回 ProgId 对应的打开命令。
func progIDCommand(progID string) string {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, progID+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	cmd, _, err := k.GetStringValue("")
	if err != nil {
		return ""
	}
	return cmd
}

// defaultBrowser 返回系统默认浏览器。
func defaultBrowser() (Candidate, bool) {
	progID := userChoiceProgID()
	if progID == "" {
		return Candidate{}, false
	}
	exe := parseCommandExecutable(progIDCommand(progID))
	if exe == "" {
		return Candidate{}, false
	}
	if info, err := os.Stat(exe); err != nil || info.IsDir() {
		return Candidate{}, false
	}
	return Candidate{
		Name:    browserName(exe),
		Path:    exe,
		Method:  "windows-userchoice",
		Default: true,
		CDP:     chromiumByPath(exe),
	}, true
}

// parseCommandExecutable 从命令行里取出可执行文件路径。
// 注册表里的命令行通常是 "\"C:\Path\app.exe\" --single-argument %1"。
func parseCommandExecutable(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if expanded, err := registry.ExpandString(command); err == nil {
		command = expanded
	}
	var path string
	if strings.HasPrefix(command, `"`) {
		if i := strings.Index(command[1:], `"`); i >= 0 {
			path = command[1 : 1+i]
		}
	} else {
		// 没有引号时按第一个空格切分；带空格的路径通常都有引号。
		path = strings.Fields(command)[0]
	}
	return strings.TrimSpace(path)
}

// fallbackCandidates 返回本机已知的 Chromium 浏览器。Windows 上 Edge 随系统
// 安装，Chrome 大多也装了；先查注册表里的 App Paths，再查常见目录。
func fallbackCandidates() []Candidate {
	names := []string{"chrome.exe", "msedge.exe", "brave.exe", "centbrowser.exe", "vivaldi.exe"}
	var out []Candidate
	for _, name := range names {
		if path := appPathsExecutable(name); path != "" {
			out = appendCandidate(out, Candidate{
				Name:   browserName(path),
				Path:   path,
				Method: "fallback",
				CDP:    chromiumByPath(path),
			})
		}
	}
	for _, path := range commonBrowserPaths() {
		out = appendCandidate(out, Candidate{
			Name:   browserName(path),
			Path:   path,
			Method: "fallback",
			CDP:    chromiumByPath(path),
		})
	}
	return out
}

// appPathsExecutable 读取注册表 App Paths 中登记的可执行文件路径。
// 64 位和 32 位视图都查一遍。
func appPathsExecutable(name string) string {
	views := []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY}
	for _, view := range views {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE,
			`SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+name,
			registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("")
		k.Close()
		if err == nil && v != "" {
			if info, statErr := os.Stat(v); statErr == nil && !info.IsDir() {
				return v
			}
		}
	}
	return ""
}

// commonBrowserPaths 返回常见安装目录下的浏览器路径。
func commonBrowserPaths() []string {
	var dirs []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if v := os.Getenv(env); v != "" {
			dirs = append(dirs, v)
		}
	}
	rel := []string{
		filepath.Join("Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join("Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join("CentBrowser", "Application", "centbrowser.exe"),
		filepath.Join("Vivaldi", "Application", "vivaldi.exe"),
	}
	var out []string
	for _, dir := range dirs {
		for _, r := range rel {
			out = append(out, filepath.Join(dir, r))
		}
	}
	return out
}
