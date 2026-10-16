package cli

// ui-core 子命令：桌面客户端启动的子进程。在 127.0.0.1 随机端口上提供带令牌的
// HTTP 服务（见 uibridge 包），启动后向标准输出打印一行 JSON
// {"addr":"127.0.0.1:端口","token":"令牌"}；标准输入关闭（客户端退出）时自行退出。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/uibridge"
)

func runUICore(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ui-core", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "未知参数：%s\n", fs.Arg(0))
		return exitUsage
	}
	socket, err := paths.SocketPath()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	token, err := uibridge.NewToken()
	if err != nil {
		fmt.Fprintf(stderr, "生成令牌失败：%v\n", err)
		return exitError
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(stderr, "监听本地端口失败：%v\n", err)
		return exitError
	}
	srv := &http.Server{Handler: uibridge.Handler(socket, token), ReadHeaderTimeout: 30 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// 客户端退出（含崩溃）时管道关闭，桥接跟着退出，不留孤儿进程。
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	line, _ := json.Marshal(map[string]string{"addr": ln.Addr().String(), "token": token})
	fmt.Fprintln(stdout, string(line))
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "服务中断：%v\n", err)
		return exitError
	}
	return exitOK
}
