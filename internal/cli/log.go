package cli

// log 子命令：查询操作记录（本机、指定设备或所有在线设备合并）。

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

const logHelp = `查询操作记录：每次工具调用都会记在被控设备上（谁、什么时候、调了什么工具、
参数摘要和结果）；发出调用的设备也记一份。不指定 -d 时查本机（self）；
--all 向所有在线设备查询并按时间合并，同一次调用只显示一条。参数摘要里的
密码、令牌、文件内容等已脱敏。离线设备的记录要等它上线后才能看到。`

func runLog(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("log", "log [-d 设备 | --all] [选项]", logHelp, true, stdout, stderr)
	all := t.fs.Bool("all", false, "查询所有在线设备并合并")
	limit := t.fs.Int("n", 50, "最多显示多少条")
	ai := t.fs.Bool("ai", false, "只看 AI 发起的")
	human := t.fs.Bool("human", false, "只看人发起的")
	tool := t.fs.String("tool", "", "按工具名筛选，以 * 结尾为前缀，如 fs.*")
	status := t.fs.String("status", "", "按结果筛选：running、ok、failed 或具体错误码")
	role := t.fs.String("role", "", "只看一端的记录：target（被控端）或 source（控制端）；不能和 --all 同用")
	since := t.fs.Duration("since", 0, "只看最近一段时间内的，如 1h、30m")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 0 {
		return t.usageError("log 不接受位置参数")
	}
	if *ai && *human {
		return t.usageError("--ai 和 --human 不能同时使用")
	}
	if *all && *role != "" {
		return t.usageError("--all 不能和 --role 同用")
	}
	if *all && *t.device != "" {
		return t.usageError("--all 不能和 -d 同用")
	}
	if *role != "" && *role != protocol.RoleTarget && *role != protocol.RoleSource {
		return t.usageError("--role 只能是 target 或 source")
	}
	if *limit <= 0 {
		return t.usageError("-n 必须大于 0")
	}
	c, err := localClient()
	if err != nil {
		return t.fail(fmt.Errorf("确定本机接口路径失败：%w", err))
	}
	opType := ""
	switch {
	case *ai:
		opType = protocol.InitiatorAI
	case *human:
		opType = protocol.InitiatorHuman
	}
	var sinceMS int64
	if *since > 0 {
		sinceMS = time.Now().Add(-*since).UnixMilli()
	}
	ctx := context.Background()

	var items []client.ActivityItem
	var failed map[string]string
	if *all {
		res, err := c.Activity(ctx, client.ActivityQuery{Since: sinceMS, OperatorType: opType, Tool: *tool, Status: *status, Limit: *limit})
		if err != nil {
			return t.fail(err)
		}
		items, failed = res.Items, res.Errors
	} else {
		dev := *t.device
		if dev == "" {
			dev = os.Getenv(envDevice)
		}
		if dev == "" {
			dev = "self"
		}
		resp, err := c.Records(ctx, dev, client.RecordQuery{
			Since: sinceMS, OperatorType: opType, Tool: *tool, Status: *status, Role: *role, Limit: *limit,
		})
		if err != nil {
			return t.fail(err)
		}
		for _, r := range resp.Records {
			it := client.ActivityItem{Record: r, TargetName: r.Device, SourceDevice: r.CallerDevice}
			items = append(items, it)
		}
	}

	if *t.jsonOut {
		printJSON(stdout, map[string]any{"items": items, "errors": failed})
		return exitOK
	}
	for name, why := range failed {
		fmt.Fprintf(stderr, "设备 %s 查询失败：%s\n", name, why)
	}
	if len(items) == 0 {
		fmt.Fprintln(stdout, "没有符合条件的记录")
		return exitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "时间\t设备\t操作者\t工具\t结果\t耗时\t参数")
	for _, it := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			time.UnixMilli(it.StartedAt).Format("01-02 15:04:05"),
			dash(it.TargetName), operatorText(it.Record), it.Tool, statusText(it.Record),
			durationText(it.Record), oneLine(it.Args, 60))
	}
	tw.Flush()
	return exitOK
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// operatorText 如“AI/mcp”“人/cli”。
func operatorText(r protocol.Record) string {
	who := "人"
	if r.OperatorType == protocol.InitiatorAI {
		who = "AI"
	}
	if r.Via != "" {
		who += "/" + r.Via
	}
	return who
}

func statusText(r protocol.Record) string {
	if r.Status == "running" {
		return "进行中"
	}
	return r.Status
}

func durationText(r protocol.Record) string {
	if r.EndedAt == 0 {
		return "-"
	}
	return (time.Duration(r.DurationMS) * time.Millisecond).String()
}

// oneLine 把文本压成一行并截到 max 个字符。
func oneLine(s string, max int) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
