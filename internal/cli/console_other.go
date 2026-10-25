//go:build !windows

package cli

import (
	"fmt"
	"io"
)

// runConsoleWindow 只在 Windows 上实现：可见窗口由 console 子命令持有。
func runConsoleWindow(o consoleOptions, stdout, stderr io.Writer) (int, error) {
	return exitError, fmt.Errorf("可见控制台窗口只在 Windows 上支持")
}
