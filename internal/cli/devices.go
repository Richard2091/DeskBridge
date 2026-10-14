package cli

// devices 子命令：列出各连接方式下的设备。

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
)

const devicesUsage = "devices [--refresh] [--json]"

const devicesHelp = `列出各连接方式下的设备，按连接方式分组。每行依次是设备名称（没有 DeskBridge
时用内网名）、内网名、系统、在线状态、路径、延迟、DeskBridge 版本或客户端类型。
-d 的设备可以写这里的设备名称、内网名或内网 IP。`

func runDevices(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("devices", devicesUsage, devicesHelp, true, stdout, stderr)
	refresh := t.fs.Bool("refresh", false, "忽略节点的探测缓存，重新探测各设备")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 0 {
		return t.usageError("devices 不接受位置参数")
	}
	c, err := localClient()
	if err != nil {
		return t.fail(fmt.Errorf("确定本机接口路径失败：%w", err))
	}
	// 探测各设备可能要几秒，给足时间。
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	list, err := c.Devices(ctx, *refresh)
	if err != nil {
		return t.fail(err)
	}
	if *t.jsonOut {
		printJSON(stdout, list)
		return exitOK
	}
	printDevices(stdout, list)
	return exitOK
}

// printDevices 按连接方式分组输出设备列表。
func printDevices(w io.Writer, list *client.DeviceList) {
	for i, g := range list.Groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, groupHeader(g))
		if g.Error != "" {
			fmt.Fprintf(w, "  读取设备列表失败：%s\n", g.Error)
		}
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, d := range g.Devices {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				deviceTitle(d), d.Name, orDash(d.OS), onlineText(d), pathText(deviceAsPeer(d)),
				latencyText(d), deskbridgeText(d))
		}
		tw.Flush()
	}
}

// groupHeader 返回分组标题，如“Tailscale（已连接，账号 alice）”。
func groupHeader(g client.DeviceGroup) string {
	name := string(g.Kind)
	if g.Kind == mesh.KindTailscale {
		name = "Tailscale"
	}
	state := stateText(g.Status.State)
	if g.Status.Account != "" {
		state += "，账号 " + g.Status.Account
	}
	if g.Status.Reason != "" {
		state += "，" + g.Status.Reason
	}
	return fmt.Sprintf("%s（%s）", name, state)
}

func stateText(s mesh.State) string {
	switch s {
	case mesh.StateStarting:
		return "启动中"
	case mesh.StateNeedsLogin:
		return "需要登录"
	case mesh.StateConnected:
		return "已连接"
	case mesh.StateDisconnected:
		return "已断开"
	case mesh.StateStopped:
		return "未启动"
	default:
		return string(s)
	}
}

// deviceTitle 返回设备名称，本机加“（本机）”。
func deviceTitle(d client.Device) string {
	if d.Self {
		return d.DisplayName() + "（本机）"
	}
	return d.DisplayName()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func onlineText(d client.Device) string {
	if d.Online {
		return "在线"
	}
	return "离线"
}

// deviceAsPeer 取出 pathText 需要的路径字段。
func deviceAsPeer(d client.Device) mesh.Peer {
	return mesh.Peer{Path: d.Path, Relay: d.Relay}
}

func latencyText(d client.Device) string {
	if d.LatencyMS <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f 毫秒", d.LatencyMS)
}

// deskbridgeText 返回 DeskBridge 一列：版本，或没有 DeskBridge 时的说明。
func deskbridgeText(d client.Device) string {
	if d.DeskBridge != nil {
		return "DeskBridge " + d.DeskBridge.Version
	}
	var text string
	switch d.Client {
	case mesh.ClientTailscale:
		text = "Tailscale 客户端"
	case mesh.ClientTSNet:
		text = "其他 tsnet 程序"
	default:
		text = "未运行 DeskBridge"
	}
	if d.ProbeError != "" {
		text += "（" + strings.TrimSpace(d.ProbeError) + "）"
	}
	return text
}
