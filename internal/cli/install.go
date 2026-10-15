package cli

// install / uninstall 子命令：管理节点的登录自启动。

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Richard2091/DeskBridge/internal/autostart"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/platform"
)

func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	taskName := fs.String("task-name", autostart.DefaultTaskName, "计划任务名")
	name := fs.String("name", "", "设备显示名，传给 node")
	hostname := fs.String("hostname", "", "内网主机名，传给 node")
	controlURL := fs.String("control-url", "", "控制服务器地址，传给 node")
	port := fs.Int("port", 7465, "节点协议端口，传给 node")
	noStart := fs.Bool("no-start", false, "只安装，不立即启动（默认立即启动，不用等下次登录）")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "未知参数：%s（预授权密钥不能写进自启动项，请先运行 node 或 login 登录一次）\n", fs.Arg(0))
		return exitUsage
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.Abs(exe)
	}
	if err != nil {
		fmt.Fprintf(stderr, "确定程序路径失败：%v\n", err)
		return exitError
	}
	logPath, err := paths.NodeLogFile()
	if err != nil {
		fmt.Fprintf(stderr, "确定日志路径失败：%v\n", err)
		return exitError
	}
	nodeArgs := []string{"node", "-log-file", logPath, "-hide-console", "-port", fmt.Sprint(*port)}
	for _, o := range [][2]string{{"-name", *name}, {"-hostname", *hostname}, {"-control-url", *controlURL}} {
		if o[1] != "" {
			nodeArgs = append(nodeArgs, o[0], o[1])
		}
	}
	err = autostart.Install(autostart.Config{TaskName: *taskName, Exe: exe, Args: nodeArgs})
	if err != nil {
		return installFailed(stderr, "安装", err)
	}
	fmt.Fprintf(stdout, "已安装计划任务 %s：用户登录后自动运行 %s\n日志：%s\n", *taskName, exe, logPath)
	fmt.Fprintln(stdout, "更换程序文件时需先 uninstall 或结束任务，替换后再 install；路径不变时只需重新运行任务。")
	if *noStart {
		return exitOK
	}
	if err := autostart.Start(*taskName); err != nil {
		return installFailed(stderr, "启动", err)
	}
	fmt.Fprintln(stdout, "已启动。")
	return exitOK
}

func runUninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	taskName := fs.String("task-name", autostart.DefaultTaskName, "计划任务名")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "未知参数：%s\n", fs.Arg(0))
		return exitUsage
	}
	if err := autostart.Uninstall(*taskName); err != nil {
		return installFailed(stderr, "卸载", err)
	}
	fmt.Fprintf(stdout, "已卸载计划任务 %s（正在运行的节点已结束）。\n", *taskName)
	return exitOK
}

func installFailed(stderr io.Writer, what string, err error) int {
	if errors.Is(err, platform.ErrUnsupportedPlatform) {
		fmt.Fprintf(stderr, "%s自启动：unsupported_platform（目前只支持 Windows）\n", what)
	} else {
		fmt.Fprintf(stderr, "%s自启动失败：%v\n", what, err)
	}
	return exitError
}
