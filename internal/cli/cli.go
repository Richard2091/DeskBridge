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
		{"node", "作为节点运行：加入内网，对外提供工具", runNode},
		{"install", "安装自启动：用户登录后自动运行节点（Windows 计划任务）", runInstall},
		{"uninstall", "卸载自启动并结束正在运行的节点", runUninstall},
		{"status", "查看本机节点和各连接方式的状态", runStatus},
		{"login", "登录一个连接方式（默认 tailscale）", runLogin},
		{"logout", "退出一个连接方式的登录", runLogout},
		{"capability", "查看对端设备提供的能力", runCapability},
		{"devices", "列出各连接方式下的设备", runDevices},
		{"ping", "测量到设备的路径和延迟，确认 DeskBridge 是否响应", runPing},
		{"info", "查看设备的系统信息", runInfo},
		{"exec", "在设备上执行命令", runExec},
		{"read", "读取设备上的文件", runRead},
		{"write", "写入设备上的文件", runWrite},
		{"list", "列出设备上的目录（别名 ls）", runList},
		{"ls", "同 list", runList},
		{"stat", "查看设备上文件或目录的信息", runStat},
		{"ps", "列出设备上的进程", runPs},
		{"kill", "结束设备上的进程", runKill},
		{"call", "通用工具调用，输出完整的响应 JSON", runCall},
		{"mcp", "作为 MCP 服务器运行，供 AI Agent 使用", runMCP},
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
