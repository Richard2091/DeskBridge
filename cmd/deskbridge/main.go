// deskbridge 是 DeskBridge 的唯一程序入口，节点、命令行客户端、MCP 服务器
// 和客户端子进程都是它的子命令。
package main

import (
	"os"

	"github.com/Richard2091/DeskBridge/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
