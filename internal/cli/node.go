package cli

// node 子命令：作为节点运行，加入内网并对外提供服务。

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Richard2091/DeskBridge/internal/deviceid"
	"github.com/Richard2091/DeskBridge/internal/logfile"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/node"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/platform"
	"github.com/Richard2091/DeskBridge/internal/store"
	"github.com/Richard2091/DeskBridge/internal/tools"
	"github.com/Richard2091/DeskBridge/internal/tools/builtin"
	"github.com/Richard2091/DeskBridge/internal/version"
)

func runNode(args []string, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("node", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "设备显示名，可以用中文，默认本机主机名")
	hostname := fs.String("hostname", "", "内网中的主机名（只能用英文、数字和连字符），默认本机主机名")
	controlURL := fs.String("control-url", "", "控制服务器地址（如自建 Headscale），默认 Tailscale 官方")
	authKey := fs.String("auth-key", "", "预授权密钥，留空则交互式登录；也可用环境变量 DESKBRIDGE_AUTH_KEY")
	port := fs.Int("port", 7465, "节点协议端口")
	logPath := fs.String("log-file", "", "日志写入的文件（按大小轮转），留空则输出到标准错误；install 安装的自启动项使用默认路径")
	hideConsole := fs.Bool("hide-console", false, "关闭控制台窗口（仅 Windows，需配合 -log-file）")
	verbose := fs.Bool("verbose", false, "输出连接层的详细日志")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "未知参数：%s\n", fs.Arg(0))
		return exitUsage
	}
	auth := *authKey
	if auth == "" {
		auth = os.Getenv("DESKBRIDGE_AUTH_KEY")
	}

	if *hideConsole && *logPath == "" {
		fmt.Fprintln(stderr, "-hide-console 需要同时指定 -log-file")
		return exitUsage
	}
	logOut := stderr
	if *logPath != "" {
		lf, err := logfile.Open(*logPath, logfile.DefaultMaxSize, logfile.DefaultBackups)
		if err != nil {
			fmt.Fprintf(stderr, "打开日志文件失败：%v\n", err)
			return exitError
		}
		defer lf.Close()
		logOut = lf
	}
	if *hideConsole {
		if err := platform.HideConsole(); err != nil {
			fmt.Fprintf(logOut, "关闭控制台窗口失败：%v\n", err)
			return exitError
		}
	}
	log.SetOutput(logOut)
	logf := func(format string, args ...any) { log.Printf(format, args...) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := runNodeCore(ctx, nodeOptions{
		Name: *name, Hostname: *hostname, ControlURL: *controlURL,
		AuthKey: auth, Port: *port, Verbose: *verbose,
	}, logf)
	if err != nil {
		return exitError
	}
	return exitOK
}

// nodeOptions 是节点运行所需的参数（node 子命令和 ui-core 内置节点共用）。
type nodeOptions struct {
	Name, Hostname, ControlURL, AuthKey string
	Port                                int
	Verbose                             bool
	OnListening                         func() // 本机接口开始监听后调用，可为 nil
}

// runNodeCore 准备本地目录并运行节点，直到 ctx 取消或出错。
// ctx 取消属于正常退出，返回 nil；其他失败已写入日志并返回错误。
func runNodeCore(ctx context.Context, opts nodeOptions, logf func(string, ...any)) error {
	_, err := paths.EnsureHome()
	var tsDir string
	if err == nil {
		tsDir, err = paths.TailscaleDir()
	}
	if err == nil {
		err = paths.Ensure(tsDir)
	}
	if err != nil {
		logf("准备本地目录失败：%v", err)
		return err
	}
	keyFile, err := paths.DeviceKeyFile()
	if err != nil {
		logf("确定设备密钥路径失败：%v", err)
		return err
	}
	dev, err := deviceid.Load(deviceid.DefaultStore(keyFile))
	if err != nil {
		logf("加载设备 ID 失败：%v", err)
		return err
	}
	logf("设备 ID：%s", dev.ID())

	var tsLogf func(format string, args ...any)
	if opts.Verbose {
		tsLogf = logf
	}
	ch := mesh.NewTailscale(mesh.TailscaleConfig{
		Dir:        tsDir,
		Hostname:   opts.Hostname,
		ControlURL: opts.ControlURL,
		AuthKey:    opts.AuthKey,
		Logf:       tsLogf,
	})

	socketPath, err := paths.SocketPath()
	if err != nil {
		logf("确定本机接口路径失败：%v", err)
		return err
	}

	// 设备显示名和 node.New 的默认规则一致：未指定时用主机名，
	// sys.info 的 name 字段也用它。
	displayName := opts.Name
	if displayName == "" {
		if h, herr := os.Hostname(); herr == nil && h != "" {
			displayName = h
		}
	}
	registry := tools.NewRegistry()
	if err := builtin.Register(registry, builtin.Info{Name: displayName, Version: version.Version}); err != nil {
		logf("注册内置工具失败：%v", err)
		return err
	}

	var records *store.DB
	if recPath, rerr := paths.RecordsFile(); rerr != nil {
		logf("确定操作记录路径失败，本次运行不记录操作：%v", rerr)
	} else if records, rerr = store.Open(recPath); rerr != nil {
		logf("打开操作记录失败，本次运行不记录操作：%v", rerr)
		records = nil
	} else {
		defer records.Close()
	}

	n := node.New(node.Config{
		Channels:    []mesh.Channel{ch},
		Device:      dev,
		Tools:       registry,
		Records:     records,
		Name:        opts.Name,
		Port:        opts.Port,
		Socket:      socketPath,
		Logf:        logf,
		OnListening: opts.OnListening,
	})
	logf("节点启动中（Ctrl+C 退出）")
	err = n.Run(ctx)
	ch.Close()
	if err != nil && errors.Is(err, context.Canceled) {
		err = nil
	}
	if err != nil {
		logf("节点异常退出：%v", err)
		return err
	}
	logf("节点已退出")
	return nil
}
