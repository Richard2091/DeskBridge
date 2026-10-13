package cli

// status、login、logout、capability 子命令：经本机接口调用节点。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/paths"
)

// localClient 创建连接本机节点的客户端；路径确定失败时返回错误。
func localClient() (*client.Client, error) {
	socketPath, err := paths.SocketPath()
	if err != nil {
		return nil, err
	}
	return client.New(socketPath), nil
}

// reportNotRunning 对"节点未运行"错误给出提示，返回是否处理了该错误。
func reportNotRunning(stderr io.Writer, err error) bool {
	if !errors.Is(err, client.ErrNotRunning) {
		return false
	}
	fmt.Fprintln(stderr, "本机节点未运行，先运行 deskbridge node")
	return true
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "以 JSON 格式输出")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	c, err := localClient()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := c.Status(ctx)
	if err != nil {
		if !reportNotRunning(stderr, err) {
			fmt.Fprintf(stderr, "获取状态失败：%v\n", err)
		}
		return exitError
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st); err != nil {
			fmt.Fprintf(stderr, "输出失败：%v\n", err)
			return exitError
		}
		return exitOK
	}
	fmt.Fprintf(stdout, "设备 ID：%s\n名称：%s\n版本：%s\n", st.DeviceID, st.Name, st.Version)
	for _, ch := range st.Channels {
		fmt.Fprintf(stdout, "\n%s：%s", ch.Kind, ch.State)
		if ch.Account != "" {
			fmt.Fprintf(stdout, "（账号 %s）", ch.Account)
		}
		if ch.Reason != "" {
			fmt.Fprintf(stdout, "，%s", ch.Reason)
		}
		fmt.Fprintln(stdout)
		if ch.Self != nil && len(ch.Self.Addrs) > 0 {
			fmt.Fprintf(stdout, "  本机内网地址：%s\n", ch.Self.Addrs[0])
		}
		if ch.LoginURL != "" {
			fmt.Fprintf(stdout, "  登录链接：%s\n", ch.LoginURL)
		}
	}
	return exitOK
}

func runLogin(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "以 JSON 格式输出")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	c, err := localClient()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := c.Login(ctx, string(mesh.KindTailscale))
	if err != nil {
		if !reportNotRunning(stderr, err) {
			fmt.Fprintf(stderr, "登录失败：%v\n", err)
		}
		return exitError
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		_ = enc.Encode(st)
		return exitOK
	}
	if st.State == mesh.StateConnected {
		fmt.Fprintf(stdout, "已登录（%s）\n", st.Account)
		return exitOK
	}
	if st.LoginURL != "" {
		fmt.Fprintf(stdout, "在浏览器中打开以下链接登录 Tailscale：\n  %s\n", st.LoginURL)
	} else {
		fmt.Fprintf(stdout, "当前状态：%s\n", st.State)
	}
	return exitOK
}

func runLogout(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	c, err := localClient()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Logout(ctx, string(mesh.KindTailscale)); err != nil {
		if !reportNotRunning(stderr, err) {
			fmt.Fprintf(stderr, "退出登录失败：%v\n", err)
		}
		return exitError
	}
	fmt.Fprintln(stdout, "已退出登录")
	return exitOK
}

func runCapability(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("capability", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "用法：deskbridge capability <设备>")
		return exitUsage
	}
	c, err := localClient()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := c.Get(ctx, fs.Arg(0), "/v1/capability")
	if err != nil {
		if !reportNotRunning(stderr, err) {
			fmt.Fprintf(stderr, "查询失败：%v\n", err)
		}
		return exitError
	}
	fmt.Fprintln(stdout, string(data))
	return exitOK
}
