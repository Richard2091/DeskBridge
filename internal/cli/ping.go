package cli

// ping 子命令：测量到设备的路径和延迟，确认对端 DeskBridge 是否响应。

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/mesh"
)

const pingUsage = "ping [--json] <设备>"

const pingHelp = `测量到设备的网络路径和延迟，并请求对端的 /v1/capability 确认 DeskBridge 是否响应。
设备写法同 -d（名称、主机名、内网 IP、连接方式:设备、self）。选项必须写在设备之前。
对端 DeskBridge 没有响应时退出码为 1。`

func runPing(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("ping", pingUsage, pingHelp, true, stdout, stderr)
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 1 {
		return t.usageError("需要且只需要一个设备")
	}
	target := t.fs.Arg(0)
	c, err := localClient()
	if err != nil {
		return t.fail(fmt.Errorf("确定本机接口路径失败：%w", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := c.Probe(ctx, target)
	if err != nil {
		return t.fail(err)
	}
	if *t.jsonOut {
		printJSON(stdout, r)
	} else {
		fmt.Fprintln(stdout, pingText(target, r))
	}
	if !r.DeskBridge {
		return exitError
	}
	return exitOK
}

// pingText 生成 ping 的文本输出，如
// “desktop：中继 hkg，延迟 231 毫秒；DeskBridge 响应 245 毫秒”。
func pingText(target string, r client.ProbeResult) string {
	if r.Self {
		return fmt.Sprintf("%s：本机，不经过内网", target)
	}
	name := r.Peer.Name
	if name == "" {
		name = target
	}
	text := name + "：" + pathText(r.Peer)
	if r.Peer.LatencyMS > 0 {
		text += fmt.Sprintf("，延迟 %.0f 毫秒", r.Peer.LatencyMS)
	}
	if r.DeskBridge {
		text += fmt.Sprintf("；DeskBridge 响应 %.0f 毫秒", r.CapabilityMS)
	} else {
		text += "；DeskBridge 无响应"
	}
	if r.Error != "" {
		text += "（" + r.Error + "）"
	}
	return text
}

// pathText 返回路径的中文描述。
func pathText(p mesh.Peer) string {
	switch p.Path {
	case mesh.PathLAN:
		return "局域网直连"
	case mesh.PathDirect:
		return "跨网直连"
	case mesh.PathRelay:
		if p.Relay != "" {
			return "中继 " + p.Relay
		}
		return "中继"
	default:
		return "路径未知"
	}
}
