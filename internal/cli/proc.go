package cli

// info、ps、kill、call 子命令：系统信息、进程管理和通用工具调用。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// infoKeyOrder 是 info 文本输出中已知键的顺序，其余键按字母序排在后面。
var infoKeyOrder = []string{
	"name", "hostname", "os", "arch", "os_version", "version", "user", "home",
	"num_cpu", "memory_total_bytes", "memory_available_bytes", "boot_time",
	"timezone", "codepages", "session",
}

func runInfo(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("info", "info [-d 设备] [--json]", "查看目标设备的系统信息（sys.info）。", true, stdout, stderr)
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 0 {
		return t.usageError("info 不接受位置参数")
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}
	var r map[string]json.RawMessage
	return s.finish("sys.info", nil, &r, func() {
		seen := make(map[string]bool, len(r))
		var rest []string
		for k := range r {
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range append(append([]string{}, infoKeyOrder...), rest...) {
			v, ok := r[k]
			if !ok || seen[k] {
				continue
			}
			seen[k] = true
			fmt.Fprintf(stdout, "%s：%s\n", k, infoValue(v))
		}
	})
}

// infoValue 把 JSON 值转成文本：字符串去掉引号，其余保持紧凑 JSON。
func infoValue(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	return string(raw)
}

// procEntry 是 proc.list 的一个进程。
type procEntry struct {
	PID         int    `json:"pid"`
	PPID        int    `json:"ppid"`
	Name        string `json:"name"`
	MemoryBytes int64  `json:"memory_bytes"`
}

// procListResult 是 proc.list 的返回值。
type procListResult struct {
	Processes []procEntry `json:"processes"`
	Truncated bool        `json:"truncated"`
}

func runPs(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("ps", "ps [-d 设备] [--name 关键字] [--max N] [--json]", "列出目标设备上的进程（proc.list）。", true, stdout, stderr)
	name := t.fs.String("name", "", "只列出进程名包含该字符串的进程（不区分大小写）")
	maxEntries := t.fs.Int("max", 0, "最多列出的进程数，0 用对端默认值（500，上限 10000）")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 0 {
		return t.usageError("ps 不接受位置参数")
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}
	callArgs := map[string]any{}
	if *name != "" {
		callArgs["name"] = *name
	}
	if *maxEntries > 0 {
		callArgs["max_entries"] = *maxEntries
	}
	var r procListResult
	return s.finish("proc.list", callArgs, &r, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "PID\tPPID\t内存(MB)\t名称")
		for _, p := range r.Processes {
			mem := "-"
			if p.MemoryBytes > 0 {
				mem = strconv.FormatFloat(float64(p.MemoryBytes)/(1<<20), 'f', 1, 64)
			}
			fmt.Fprintf(tw, "%d\t%d\t%s\t%s\n", p.PID, p.PPID, mem, p.Name)
		}
		tw.Flush()
		if r.Truncated {
			fmt.Fprintln(stderr, "提示：进程过多，列表已被截断，可用 --max 调大上限")
		}
	})
}

func runKill(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("kill", "kill [-d 设备] [--tree] [--force] [--json] <pid>", "结束目标设备上的进程（proc.kill）。选项必须写在 pid 之前。", true, stdout, stderr)
	tree := t.fs.Bool("tree", false, "连同子进程一起结束")
	force := t.fs.Bool("force", false, "强制结束（Linux 发 SIGKILL，Windows 总是强制）")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 1 {
		return t.usageError("需要且只需要一个 pid")
	}
	pid, err := strconv.Atoi(t.fs.Arg(0))
	if err != nil || pid <= 0 {
		return t.usageError("pid 必须是正整数：%q", t.fs.Arg(0))
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}
	var r struct {
		Killed []int `json:"killed"`
	}
	return s.finish("proc.kill", map[string]any{"pid": pid, "tree": *tree, "force": *force}, &r, func() {
		ids := make([]string, len(r.Killed))
		for i, k := range r.Killed {
			ids[i] = strconv.Itoa(k)
		}
		fmt.Fprintf(stdout, "已结束进程：%s\n", strings.Join(ids, "、"))
	})
}

const callUsage = "call [-d 设备] --tool <名称> [--args '<json>'] [--timeout 时长]"

func runCall(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("call", callUsage, "通用工具调用，总是输出完整的 CallResponse JSON（同 agentdesk call）。", false, stdout, stderr)
	tool := t.fs.String("tool", "", "工具名，如 proc.exec")
	argsJSON := t.fs.String("args", "", "工具参数，JSON 对象，默认不带参数")
	timeout := t.fs.Duration("timeout", 0, "调用超时，0 用工具默认值")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if *tool == "" {
		return t.usageError("缺少 --tool")
	}
	if t.fs.NArg() != 0 {
		return t.usageError("call 不接受位置参数，参数用 --args 传入")
	}
	var callArgs json.RawMessage
	if *argsJSON != "" {
		if !json.Valid([]byte(*argsJSON)) {
			return t.usageError("--args 不是合法的 JSON")
		}
		callArgs = json.RawMessage(*argsJSON)
	}
	if *timeout < 0 {
		return t.usageError("--timeout 不能为负")
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}
	var in any
	if callArgs != nil {
		in = callArgs
	}
	resp, err := s.call(context.Background(), *tool, in, client.CallOptions{TimeoutMS: int(timeout.Milliseconds())})
	if resp != nil {
		// 工具返回的失败也输出完整响应，退出码仍为 1。
		printJSON(stdout, resp)
		if err != nil {
			return exitError
		}
		return exitOK
	}
	// 没有收到工具响应（设备不存在、节点未运行等）：输出同样结构的失败响应。
	printJSON(stdout, protocol.CallResponse{OK: false, Error: errorBody(err)})
	return exitError
}
