package cli

// ui-core 子命令：桌面客户端启动的子进程。在 127.0.0.1 随机端口上提供带令牌的
// HTTP 服务（见 uibridge 包），启动后向标准输出打印一行 JSON
// {"addr":"127.0.0.1:端口","token":"令牌","node":"external|embedded|off"}；
// 标准输入关闭（客户端退出）时自行退出。-node=auto（默认）时，本机已有节点就复用，
// 没有就在本进程内运行节点（embedded），退出时一起关闭。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Richard2091/DeskBridge/internal/logfile"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/uibridge"
)

func runUICore(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ui-core", flag.ContinueOnError)
	fs.SetOutput(stderr)
	nodeFlag := fs.String("node", "auto", "本机节点：auto 已有则复用、没有则在本进程内运行；off 只转发")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "未知参数：%s\n", fs.Arg(0))
		return exitUsage
	}
	if *nodeFlag != "auto" && *nodeFlag != "off" {
		fmt.Fprintf(stderr, "-node 只能是 auto 或 off，收到：%s\n", *nodeFlag)
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

	mode := "off"
	var nodeDone <-chan struct{}
	if *nodeFlag == "auto" {
		if nodeReachable(socket, time.Second) {
			mode = "external"
		} else {
			mode = "embedded"
			var logPath string
			nodeDone, logPath, err = startEmbeddedNode(ctx, socket, stderr)
			if err != nil {
				fmt.Fprintf(stderr, "本机节点启动失败：%v（日志：%s）\n", err, logPath)
				stop()
				if nodeDone != nil {
					waitDone(nodeDone, embeddedExitWait)
				}
				return exitError
			}
		}
	}

	go func() {
		// 客户端退出（含崩溃）时管道关闭，桥接跟着退出，不留孤儿进程。
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	line, _ := json.Marshal(map[string]string{"addr": ln.Addr().String(), "token": token, "node": mode})
	fmt.Fprintln(stdout, string(line))
	code := exitOK
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "服务中断：%v\n", err)
		code = exitError
	}
	stop()
	if nodeDone != nil {
		waitDone(nodeDone, embeddedExitWait)
	}
	return code
}

const (
	embeddedReadyWait = 15 * time.Second
	embeddedExitWait  = 10 * time.Second
)

// waitDone 等 done 关闭，最多等 d。
func waitDone(done <-chan struct{}, d time.Duration) {
	select {
	case <-done:
	case <-time.After(d):
	}
}

// nodeReachable 报告能否在 timeout 内连上本机接口 socket。
func nodeReachable(socket string, timeout time.Duration) bool {
	c, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// startEmbeddedNode 在 goroutine 里运行节点并等本机接口就绪。
// 返回的 done 在节点退出后关闭（失败时也返回，便于调用方等待收尾）。
func startEmbeddedNode(ctx context.Context, socket string, stderr io.Writer) (<-chan struct{}, string, error) {
	dir, err := paths.LogDir()
	if err != nil {
		return nil, "", fmt.Errorf("确定日志目录失败：%w", err)
	}
	logPath := filepath.Join(dir, "ui-node.log")
	lf, err := logfile.Open(logPath, logfile.DefaultMaxSize, logfile.DefaultBackups)
	if err != nil {
		return nil, logPath, fmt.Errorf("打开日志文件失败：%w", err)
	}
	lg := log.New(lf, "", log.LstdFlags)
	logf := func(format string, args ...any) { lg.Printf(format, args...) }

	done := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		defer close(done)
		defer lf.Close()
		err := runNodeCore(ctx, nodeOptions{AuthKey: os.Getenv("DESKBRIDGE_AUTH_KEY"), Port: 7465}, logf)
		result <- err // 已在日志里记录；就绪之后没人读，缓冲区够用
	}()

	deadline := time.NewTimer(embeddedReadyWait)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-result:
			if err == nil {
				err = errors.New("节点已退出")
			}
			return done, logPath, err
		case <-deadline.C:
			return done, logPath, fmt.Errorf("等待 %d 秒仍未就绪", int(embeddedReadyWait.Seconds()))
		case <-ctx.Done():
			return done, logPath, errors.New("已取消")
		case <-tick.C:
			if nodeReachable(socket, 100*time.Millisecond) {
				return done, logPath, nil
			}
		}
	}
}
