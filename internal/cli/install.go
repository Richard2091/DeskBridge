package cli

// install / uninstall 子命令：管理节点的登录自启动。

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Richard2091/DeskBridge/internal/autostart"
	"github.com/Richard2091/DeskBridge/internal/nodeconfig"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/platform"
)

func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	taskName := fs.String("task-name", autostart.DefaultTaskName, "自启动项名称（Windows 为计划任务名，macOS 为 LaunchAgent 标签的后缀）")
	settings := addNodeSettingFlags(fs, "；保存到数据目录的 node.json，没给出的保持原值")
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
	if err := saveNodeSettings(settings.explicit()); err != nil {
		fmt.Fprintf(stderr, "保存节点设置失败：%v\n", err)
		return exitError
	}
	logPath, err := paths.NodeLogFile()
	var superLog string
	if err == nil {
		superLog, err = paths.SupervisorLogFile()
	}
	if err != nil {
		fmt.Fprintf(stderr, "确定日志路径失败：%v\n", err)
		return exitError
	}
	// 守护进程负责重启和收集崩溃输出（它的日志在 superLog），节点自己写 logPath。
	nodeArgs := []string{"supervise", "-log-file", superLog}
	if runtime.GOOS == "windows" {
		// 只有 Windows 的计划任务会给控制台程序弹出窗口；其他平台上 -hide-console 会报 unsupported_platform。
		nodeArgs = append(nodeArgs, "-hide-console")
	}
	nodeArgs = append(nodeArgs, "--", "node", "-log-file", logPath)
	err = autostart.Install(autostart.Config{TaskName: *taskName, Exe: exe, Args: nodeArgs})
	if err != nil {
		return installFailed(stderr, "安装", err)
	}
	fmt.Fprintf(stdout, "已安装自启动项 %s：用户登录后自动运行 %s\n日志：%s（节点）、%s（守护进程，含崩溃输出）\n", *taskName, exe, logPath, superLog)
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

// saveNodeSettings 把显式给出的设置合并写入 node.json；一个都没给时不动文件。
func saveNodeSettings(explicit nodeconfig.Config) error {
	if explicit == (nodeconfig.Config{}) {
		return nil
	}
	if _, err := paths.EnsureHome(); err != nil {
		return err
	}
	p, err := paths.NodeConfigFile()
	if err != nil {
		return err
	}
	cur, err := nodeconfig.Load(p)
	if err != nil {
		return err
	}
	return nodeconfig.Save(p, mergeNodeConfig(cur, explicit))
}

func runUninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	taskName := fs.String("task-name", autostart.DefaultTaskName, "自启动项名称（Windows 为计划任务名，macOS 为 LaunchAgent 标签的后缀）")
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
	fmt.Fprintf(stdout, "已卸载自启动项 %s（正在运行的节点已结束）。\n", *taskName)
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
