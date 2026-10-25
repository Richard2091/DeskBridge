//go:build windows

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetConsoleTitleW = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleTitleW")
	procWriteConsoleW    = windows.NewLazySystemDLL("kernel32.dll").NewProc("WriteConsoleW")
)

// runConsoleWindow 在当前控制台窗口里运行命令：先把窗口标题设好、打印一行
// 模拟人工输入的命令，再把控制台交给命令，命令的输出直接进窗口；命令结束
// 后保留窗口 hold 一段时间。
//
// 控制台窗口是启动这个进程时新建的（proc.exec 用 CREATE_NEW_CONSOLE），但
// 标准输出和标准错误可能仍被父进程接成了空设备，所以这里直接打开控制台
// 本身（CONOUT$/CONIN$），不依赖继承来的句柄。
func runConsoleWindow(o consoleOptions, stdout, stderr io.Writer) (int, error) {
	out, in := openConsole()
	if out != nil {
		defer out.Close()
	}
	if in != nil {
		defer in.Close()
	}
	if out != nil {
		setConsoleTitle(o.Title)
		if o.Line != "" {
			writeConsoleLine(out, o.Line)
		}
	}

	cmd := exec.Command(o.Exe, o.Args...)
	if o.CmdLine != "" {
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: o.CmdLine}
	}
	if out != nil {
		cmd.Stdout, cmd.Stderr = out, out
	} else {
		cmd.Stdout, cmd.Stderr = stdout, stderr
	}
	if in != nil {
		cmd.Stdin = in
	}
	if err := cmd.Start(); err != nil {
		return exitError, err
	}
	code, err := waitExitCode(cmd)
	if err != nil {
		return exitError, err
	}
	if out != nil && o.Hold > 0 {
		writeConsoleLine(out, fmt.Sprintf("[DeskBridge] 命令结束，退出码 %d，窗口 %s 后关闭。", code, o.Hold.Round(time.Second)))
		time.Sleep(o.Hold)
	}
	return code, nil
}

// waitExitCode 等待命令结束并返回它的退出码。
func waitExitCode(cmd *exec.Cmd) (int, error) {
	err := cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 0, err
}

// openConsole 打开本进程控制台的输出和输入。没有控制台（比如在黑窗口里被
// 重定向、或在服务里运行）时返回 nil，由调用方退回标准流。
func openConsole() (out, in *os.File) {
	if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		out = f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		in = f
	}
	return out, in
}

// setConsoleTitle 设置控制台窗口标题；失败不影响命令执行。
func setConsoleTitle(title string) {
	if title == "" {
		return
	}
	p, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	_, _, _ = procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(p)))
}

// writeConsoleLine 用 WriteConsoleW 写一行，避免中文按控制台代码页变成乱码；
// 句柄不是控制台时退回普通写入。
func writeConsoleLine(f *os.File, text string) {
	p, err := windows.UTF16FromString(text + "\r\n")
	if err != nil {
		return
	}
	var written uint32
	r, _, _ := procWriteConsoleW.Call(f.Fd(), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)-1), uintptr(unsafe.Pointer(&written)), 0)
	if r == 0 {
		_, _ = f.WriteString(text + "\r\n")
	}
}
