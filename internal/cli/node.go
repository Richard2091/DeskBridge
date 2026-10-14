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
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/node"
	"github.com/Richard2091/DeskBridge/internal/paths"
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

	log.SetOutput(stderr)
	logf := func(format string, args ...any) { log.Printf(format, args...) }

	tsDir, err := paths.TailscaleDir()
	if err == nil {
		err = paths.Ensure(tsDir)
	}
	if err != nil {
		logf("准备本地目录失败：%v", err)
		return exitError
	}
	keyFile, err := paths.DeviceKeyFile()
	if err != nil {
		logf("确定设备密钥路径失败：%v", err)
		return exitError
	}
	dev, err := deviceid.Load(deviceid.DefaultStore(keyFile))
	if err != nil {
		logf("加载设备 ID 失败：%v", err)
		return exitError
	}
	logf("设备 ID：%s", dev.ID())

	var tsLogf func(format string, args ...any)
	if *verbose {
		tsLogf = logf
	}
	ch := mesh.NewTailscale(mesh.TailscaleConfig{
		Dir:        tsDir,
		Hostname:   *hostname,
		ControlURL: *controlURL,
		AuthKey:    auth,
		Logf:       tsLogf,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	socketPath, err := paths.SocketPath()
	if err != nil {
		logf("确定本机接口路径失败：%v", err)
		return exitError
	}

	// 设备显示名和 node.New 的默认规则一致：未指定时用主机名，
	// sys.info 的 name 字段也用它。
	displayName := *name
	if displayName == "" {
		if h, herr := os.Hostname(); herr == nil && h != "" {
			displayName = h
		}
	}
	registry := tools.NewRegistry()
	if err := builtin.Register(registry, builtin.Info{Name: displayName, Version: version.Version}); err != nil {
		logf("注册内置工具失败：%v", err)
		return exitError
	}

	n := node.New(node.Config{
		Channels: []mesh.Channel{ch},
		Device:   dev,
		Tools:    registry,
		Name:     *name,
		Port:     *port,
		Socket:   socketPath,
		Logf:     logf,
	})
	logf("节点启动中（Ctrl+C 退出）")
	err = n.Run(ctx)
	ch.Close()
	if err != nil && errors.Is(err, context.Canceled) {
		err = nil
	}
	if err != nil {
		logf("节点异常退出：%v", err)
		return exitError
	}
	logf("节点已退出")
	return exitOK
}
