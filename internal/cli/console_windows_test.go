//go:build windows

package cli

import (
	"io"
	"testing"
)

// TestRunConsoleWindow 验证 console 子命令把命令的退出码原样返回。
// 测试进程通常在自己的控制台里跑，此时回显和输出会写到测试控制台。
func TestRunConsoleWindow(t *testing.T) {
	code, err := runConsoleWindow(consoleOptions{
		Title:   "DeskBridge 测试",
		Line:    `C:\work> exit /b 3`,
		Exe:     "cmd.exe",
		CmdLine: `cmd.exe /d /s /c "exit /b 3"`,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runConsoleWindow 失败: %v", err)
	}
	if code != 3 {
		t.Errorf("退出码 = %d，期望 3", code)
	}
}

// TestRunConsoleWindowStartError 验证程序启动失败时返回错误。
func TestRunConsoleWindowStartError(t *testing.T) {
	if _, err := runConsoleWindow(consoleOptions{Exe: "definitely-not-a-program-deskbridge"}, io.Discard, io.Discard); err == nil {
		t.Fatal("期望启动失败")
	}
}
