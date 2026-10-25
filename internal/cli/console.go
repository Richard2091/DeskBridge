package cli

// console 子命令：proc.exec 的可见窗口模式用它在设备桌面上新建一个控制台窗口。
// 它不是给用户用的命令（不出现在帮助里）：窗口由它持有，先把命令行像人工输入
// 一样打印出来，再把控制台交给真正的命令，命令的输出直接写进窗口；命令结束
// 后窗口再保留一小段时间，方便看清退出码和最后几行输出。

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"
)

// consoleOptions 是 console 子命令的选项。
type consoleOptions struct {
	Title   string        // 窗口标题
	Line    string        // 启动前打印的一行，模拟人工输入的命令
	Exe     string        // 要运行的程序
	CmdLine string        // 直接交给 CreateProcess 的命令行；为空时执行 Args
	Args    []string      // Exe 的参数
	Hold    time.Duration // 命令结束后保留窗口的时间
}

func runConsole(args []string, stdout, stderr io.Writer) int {
	var o consoleOptions
	fs := flag.NewFlagSet("console", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.Title, "title", "DeskBridge", "控制台窗口标题")
	fs.StringVar(&o.Line, "line", "", "启动前打印的一行，模拟人工输入的命令")
	fs.StringVar(&o.Exe, "exe", "", "要运行的程序（必填）")
	fs.StringVar(&o.CmdLine, "cmdline", "", "直接交给 CreateProcess 的命令行；为空时执行 -- 后面的参数")
	fs.DurationVar(&o.Hold, "hold", 5*time.Second, "命令结束后保留窗口的时间")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法：deskbridge console -exe <程序> [选项] [-- 参数...]")
		fmt.Fprintln(stderr, "\n内部命令：proc.exec 的 visible 模式用它在设备桌面上打开控制台窗口，")
		fmt.Fprintln(stderr, "先打印一行模拟人工输入的命令，再把控制台交给指定的程序。")
		fmt.Fprintln(stderr, "\n选项：")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if o.Exe == "" {
		fmt.Fprintln(stderr, "console：缺少 -exe")
		return exitUsage
	}
	o.Args = fs.Args()
	if o.CmdLine != "" && len(o.Args) > 0 {
		fmt.Fprintln(stderr, "console：-cmdline 和 -- 参数不能同时给出")
		return exitUsage
	}
	code, err := runConsoleWindow(o, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "console：%v\n", err)
		return exitError
	}
	return code
}
