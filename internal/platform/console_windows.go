//go:build windows

package platform

import "golang.org/x/sys/windows"

var procFreeConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")

// HideConsole 脱离当前控制台，控制台窗口随之关闭（计划任务启动控制台
// 程序时会弹出窗口）。之后标准输出和标准错误不再可用，调用前应已改用
// 日志文件。窗口在进程启动到调用之间会闪现一瞬。
func HideConsole() error {
	if r, _, err := procFreeConsole.Call(); r == 0 {
		return err
	}
	return nil
}
