//go:build windows

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func schtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	// schtasks 的输出按系统代码页（中文系统是 GBK），原样放进错误里可能乱码，
	// 所以只在失败时附上，并提示以退出码为准。
	if err != nil {
		return string(out), fmt.Errorf("schtasks %s 失败: %w", args[0], err)
	}
	return string(out), nil
}

// Install 创建或覆盖计划任务。
func Install(c Config) error {
	if c.User == "" {
		c.User = os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	}
	if !filepath.IsAbs(c.Exe) {
		return fmt.Errorf("程序路径必须是绝对路径: %s", c.Exe)
	}
	f, err := os.CreateTemp("", "deskbridge-task-*.xml")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(f.Name())
	_, werr := f.Write(utf16LE(taskXML(c)))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("写入任务定义失败: %w", werr)
	}
	if out, err := schtasks("/Create", "/F", "/TN", c.name(), "/XML", f.Name()); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	return nil
}

// Uninstall 先结束正在运行的任务实例，再删除计划任务。任务不存在时返回错误。
func Uninstall(taskName string) error {
	c := Config{TaskName: taskName}
	if !Exists(c.name()) {
		return fmt.Errorf("计划任务 %s 不存在", c.name())
	}
	schtasks("/End", "/TN", c.name()) // 没在运行时会失败，忽略
	if out, err := schtasks("/Delete", "/F", "/TN", c.name()); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	return nil
}

// Start 立即运行计划任务（不等下次登录）。
func Start(taskName string) error {
	c := Config{TaskName: taskName}
	if out, err := schtasks("/Run", "/TN", c.name()); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
	}
	return nil
}

// Exists 报告计划任务是否存在。
func Exists(taskName string) bool {
	c := Config{TaskName: taskName}
	_, err := schtasks("/Query", "/TN", c.name())
	return err == nil
}
