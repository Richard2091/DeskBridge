package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Richard2091/DeskBridge/internal/mcp"
)

// runMCP 以 MCP 服务器运行：标准输入输出走协议，日志和提示只写标准错误。
// 标准输入关闭或收到中断信号时退出。
func runMCP(args []string, _, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "用法：deskbridge mcp（无参数）")
		return exitUsage
	}
	c, err := localClient()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := mcp.Serve(ctx, c, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintf(stderr, "MCP 服务器出错：%v\n", err)
		return exitError
	}
	return exitOK
}
