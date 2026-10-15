package cli

// supervise 子命令：运行本程序的另一个子命令，失败后自动重启。
// 自启动项用它守护节点。

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/Richard2091/DeskBridge/internal/logfile"
	"github.com/Richard2091/DeskBridge/internal/platform"
	"github.com/Richard2091/DeskBridge/internal/supervise"
)

func runSupervise(args []string, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("supervise", flag.ContinueOnError)
	fs.SetOutput(stderr)
	logPath := fs.String("log-file", "", "守护进程的日志，同时收集子进程的标准输出和标准错误（含崩溃堆栈）；留空则输出到标准错误")
	hideConsole := fs.Bool("hide-console", false, "关闭控制台窗口（仅 Windows，需配合 -log-file）")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法：deskbridge supervise [-log-file 路径] [-hide-console] -- <子命令> [参数]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return exitUsage
	}
	if *hideConsole && *logPath == "" {
		fmt.Fprintln(stderr, "-hide-console 需要同时指定 -log-file")
		return exitUsage
	}
	out := stderr
	if *logPath != "" {
		lf, err := logfile.Open(*logPath, logfile.DefaultMaxSize, logfile.DefaultBackups)
		if err != nil {
			fmt.Fprintf(stderr, "打开日志文件失败：%v\n", err)
			return exitError
		}
		defer lf.Close()
		out = lf
	}
	if *hideConsole {
		if err := platform.HideConsole(); err != nil {
			fmt.Fprintf(out, "关闭控制台窗口失败：%v\n", err)
			return exitError
		}
	}
	log.SetOutput(out)
	exe, err := os.Executable()
	if err != nil {
		log.Printf("确定程序路径失败：%v", err)
		return exitError
	}
	childArgs := fs.Args()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("守护进程启动：%s %v", exe, childArgs)
	_ = supervise.Run(ctx, func(ctx context.Context) (int, error) {
		cmd := exec.CommandContext(ctx, exe, childArgs...)
		cmd.Stdout, cmd.Stderr = out, out
		cmd.WaitDelay = 5 * time.Second
		supervise.Prepare(cmd)
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		if err := supervise.Attach(cmd); err != nil {
			// 没放进作业对象，守护进程被强制结束时会留下孤儿节点，所以不继续。
			cmd.Process.Kill()
			cmd.Wait()
			return 0, err
		}
		log.Printf("子进程已启动，pid %d", cmd.Process.Pid)
		err := cmd.Wait()
		if cmd.ProcessState != nil {
			return cmd.ProcessState.ExitCode(), nil
		}
		return 0, err
	}, supervise.Options{Logf: log.Printf})
	log.Printf("守护进程退出")
	return exitOK
}
