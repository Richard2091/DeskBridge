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
	"github.com/Richard2091/DeskBridge/internal/nodeconfig"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/platform"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/store"
	"github.com/Richard2091/DeskBridge/internal/tools"
	"github.com/Richard2091/DeskBridge/internal/tools/builtin"
	"github.com/Richard2091/DeskBridge/internal/version"
)

func runNode(args []string, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("node", flag.ContinueOnError)
	fs.SetOutput(stderr)
	settings := addNodeSettingFlags(fs, "；只对本次运行有效，要保存请用 install")
	authKey := fs.String("auth-key", "", "预授权密钥，留空则交互式登录；也可用环境变量 DESKBRIDGE_AUTH_KEY")
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

	file, err := loadNodeSettings()
	if err != nil {
		logf("%v", err)
		return exitError
	}
	opts := resolveNodeOptions(file, settings.explicit(), *authKey)
	opts.Verbose = *verbose

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runNodeCore(ctx, opts, logf); err != nil {
		return exitError
	}
	return exitOK
}

// defaultNodePort 是节点协议端口的默认值（协议规定值）。
const defaultNodePort = protocol.DefaultPort

// nodeSettingFlags 是 node 和 install 共用的节点设置参数。
// 只有命令行里显式给出的才算数（用 FlagSet.Visit 判断），其余取 node.json 或默认值。
type nodeSettingFlags struct {
	fs                         *flag.FlagSet
	name, hostname, controlURL *string
	port                       *int
}

func addNodeSettingFlags(fs *flag.FlagSet, note string) *nodeSettingFlags {
	return &nodeSettingFlags{
		fs:         fs,
		name:       fs.String("name", "", "设备显示名，可以用中文，默认本机主机名"+note),
		hostname:   fs.String("hostname", "", "内网中的主机名（只能用英文、数字和连字符），默认本机主机名"+note),
		controlURL: fs.String("control-url", "", "控制服务器地址（如自建 Headscale），默认 Tailscale 官方"+note),
		port:       fs.Int("port", defaultNodePort, "节点协议端口"+note),
	}
}

// explicit 返回命令行里显式给出的设置，没给出的字段为零值。
func (f *nodeSettingFlags) explicit() nodeconfig.Config {
	var c nodeconfig.Config
	f.fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "name":
			c.Name = *f.name
		case "hostname":
			c.Hostname = *f.hostname
		case "control-url":
			c.ControlURL = *f.controlURL
		case "port":
			c.Port = *f.port
		}
	})
	return c
}

// merge 用 over 里非零的字段覆盖 base。
func mergeNodeConfig(base, over nodeconfig.Config) nodeconfig.Config {
	if over.Name != "" {
		base.Name = over.Name
	}
	if over.Hostname != "" {
		base.Hostname = over.Hostname
	}
	if over.ControlURL != "" {
		base.ControlURL = over.ControlURL
	}
	if over.Port != 0 {
		base.Port = over.Port
	}
	return base
}

// loadNodeSettings 读取数据目录里的 node.json（不存在为零值）。
func loadNodeSettings() (nodeconfig.Config, error) {
	p, err := paths.NodeConfigFile()
	if err != nil {
		return nodeconfig.Config{}, err
	}
	return nodeconfig.Load(p)
}

// resolveNodeOptions 合成节点参数：命令行显式给出的 > node.json > 默认值。
// 预授权密钥只来自 authKey 或环境变量 DESKBRIDGE_AUTH_KEY，不写入文件。
func resolveNodeOptions(file, explicit nodeconfig.Config, authKey string) nodeOptions {
	c := mergeNodeConfig(file, explicit)
	if c.Port == 0 {
		c.Port = defaultNodePort
	}
	if authKey == "" {
		authKey = os.Getenv("DESKBRIDGE_AUTH_KEY")
	}
	return nodeOptions{Name: c.Name, Hostname: c.Hostname, ControlURL: c.ControlURL, AuthKey: authKey, Port: c.Port}
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
	lockPath, err := paths.NodeLockFile()
	if err != nil {
		logf("确定节点锁文件路径失败：%v", err)
		return err
	}
	logf("节点设置：名称 %q，主机名 %q，控制服务器 %q，端口 %d", opts.Name, opts.Hostname, opts.ControlURL, opts.Port)
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
		LockFile:    lockPath,
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
