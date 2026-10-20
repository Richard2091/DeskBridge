//go:build darwin

package autostart

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const labelPrefix = "io.github.richard2091."

// labelFor 返回 LaunchAgent 标签；名称为空时用 DefaultTaskName。
func labelFor(name string) string {
	if name == "" {
		name = DefaultTaskName
	}
	return labelPrefix + name
}

func plistPath(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("确定用户目录失败: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func plistString(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return "<string>" + b.String() + "</string>"
}

// launchdPlist 生成 LaunchAgent 定义。
//   - RunAtLoad：加载（登录或 bootstrap）时立即运行；
//   - KeepAlive.SuccessfulExit=false：只在异常退出时由 launchd 重启；
//   - LimitLoadToSessionType=Aqua：只在图形登录会话中运行；
//   - 不设 StandardOutPath：日志由 supervise 子命令写。
func launchdPlist(label string, c Config) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	` + plistString(label) + `
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range append([]string{c.Exe}, c.Args...) {
		b.WriteString("\t\t" + plistString(a) + "\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes()
}

// launchctl 运行 launchctl，失败时把输出带进错误信息。
func launchctl(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s 失败: %w\n%s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// loaded 判断该标签是否已加载到当前用户的 gui 域。
func loaded(label string) bool {
	return exec.Command("launchctl", "print", domain()+"/"+label).Run() == nil
}

// Install 写入 LaunchAgent 的 plist；已加载的同名项先卸载。不启动，由 Start 启动。
func Install(c Config) error {
	if !filepath.IsAbs(c.Exe) {
		return fmt.Errorf("程序路径必须是绝对路径: %s", c.Exe)
	}
	label := labelFor(c.TaskName)
	p, err := plistPath(label)
	if err != nil {
		return err
	}
	if loaded(label) {
		if err := launchctl("bootout", domain()+"/"+label); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}
	if err := os.WriteFile(p, launchdPlist(label, c), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", p, err)
	}
	return nil
}

// Uninstall 卸载（会结束正在运行的进程）并删除 plist。plist 不存在时返回错误。
func Uninstall(taskName string) error {
	label := labelFor(taskName)
	p, err := plistPath(label)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("LaunchAgent %s 不存在: %w", label, err)
	}
	if loaded(label) {
		if err := launchctl("bootout", domain()+"/"+label); err != nil {
			return err
		}
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("删除 %s 失败: %w", p, err)
	}
	return nil
}

// Start 未加载时 bootstrap（RunAtLoad 会立即运行），已加载时 kickstart。
func Start(taskName string) error {
	label := labelFor(taskName)
	if loaded(label) {
		return launchctl("kickstart", domain()+"/"+label)
	}
	p, err := plistPath(label)
	if err != nil {
		return err
	}
	return launchctl("bootstrap", domain(), p)
}

// Exists 报告 plist 是否存在。
func Exists(taskName string) bool {
	p, err := plistPath(labelFor(taskName))
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
