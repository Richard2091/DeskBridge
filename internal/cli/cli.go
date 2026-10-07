// Package cli 解析命令行并分派到各个子命令。
package cli

import (
	"fmt"
	"io"

	"github.com/Richard2091/DeskBridge/internal/version"
)

// 退出码。
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

type command struct {
	name    string
	summary string
	run     func(args []string, stdout, stderr io.Writer) int
}

func commands() []command {
	return []command{
		{"node", "作为节点运行：加入内网，对外提供工具", notImplemented("node")},
		{"mcp", "作为 MCP 服务器运行，供 AI Agent 使用", notImplemented("mcp")},
		{"ui-core", "供桌面客户端启动的子进程，经标准输入输出传 JSON", notImplemented("ui-core")},
		{"version", "显示版本", runVersion},
		{"help", "显示帮助", runHelp},
	}
}

// Run 执行 args 指定的子命令（不含程序名），返回进程退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "-help", "--help":
		printUsage(stdout)
		return exitOK
	case "-v", "-version", "--version":
		return runVersion(nil, stdout, stderr)
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "未知的子命令：%s\n\n", args[0])
	printUsage(stderr)
	return exitUsage
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "用法：deskbridge <子命令> [参数]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "子命令：")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
}

func runHelp(_ []string, stdout, _ io.Writer) int {
	printUsage(stdout)
	return exitOK
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "deskbridge %s\n", version.Version)
	return exitOK
}

func notImplemented(name string) func([]string, io.Writer, io.Writer) int {
	return func(_ []string, _, stderr io.Writer) int {
		fmt.Fprintf(stderr, "子命令 %s 尚未实现\n", name)
		return exitError
	}
}
