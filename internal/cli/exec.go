package cli

// exec 子命令：在目标设备上执行命令（proc.exec）。

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// 退出码：命令超时、被 Ctrl+C 中断（同 timeout 命令和 shell 的惯例）。
const (
	exitTimedOut    = 124
	exitInterrupted = 130
)

// defaultExecTimeout 是 exec 的默认超时。
const defaultExecTimeout = 60 * time.Second

// cancelTimeout 是 Ctrl+C 后请求对端取消的最长等待。
const cancelTimeout = 5 * time.Second

const execUsage = "exec [-d 设备] [选项] <命令...>"

const execHelp = `在目标设备上执行命令。选项必须写在命令之前；多个命令参数用空格拼成一行，
命令以 - 开头时先写 -- 。远程的标准输出写到本进程标准输出，标准错误写到
标准错误。--stdin 或 --stdin-file 提供的输入必须是 UTF-8 文本。

--timeout 默认 60s，上限 10m（节点会把更大的值截到 10 分钟）。耗时长的命令
不要等它：放到后台并把输出写进日志文件，再用 read 轮询，例如 Windows 上
  deskbridge exec -d win-pc --shell cmd "start /b mytask.exe > D:\log.txt 2>&1"
  deskbridge read -d win-pc D:\log.txt
（Windows 上用 start /b 启动的后台进程在命令正常结束后会保留。）

Windows 上默认不弹控制台窗口。下载、安装、构建等耗时较长、想让人在设备桌面上
看着的命令可以加 --console visible：在那个桌面上打开一个带标题的窗口，先回显
命令行再显示实时输出；此时不返回 stdout/stderr，只返回退出码。`

const execExitHelp = `退出码：远程命令的退出码（超出 0-255 时为 1）；超时 124，并提示“命令超时”；
Ctrl+C 时取消远程调用并退出 130；工具调用失败或本机节点未运行为 1；用法错误为 2。`

// envList 是可重复的 --env K=V 选项。
type envList []string

func (e *envList) String() string { return strings.Join(*e, ",") }

func (e *envList) Set(v string) error {
	if k, _, ok := strings.Cut(v, "="); !ok || k == "" {
		return fmt.Errorf("格式应为 K=V：%q", v)
	}
	*e = append(*e, v)
	return nil
}

// execResult 是 proc.exec 的返回值。
type execResult struct {
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	Console         bool   `json:"console"`
	DurationMS      int64  `json:"duration_ms"`
}

func runExec(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("exec", execUsage, execHelp, true, stdout, stderr)
	t.exitHelp = execExitHelp
	shell := t.fs.String("shell", "", "shell：powershell、pwsh、cmd、sh、bash，默认取对端平台的默认值")
	console := t.fs.String("console", "", "控制台窗口：visible 在设备桌面上打开带标题的窗口显示命令和输出（仅 Windows，不返回 stdout/stderr）")
	cwd := t.fs.String("cwd", "", "工作目录")
	var envs envList
	t.fs.Var(&envs, "env", "追加或覆盖的环境变量 K=V，可重复")
	stdinFile := t.fs.String("stdin-file", "", "把本地文件的内容作为标准输入")
	useStdin := t.fs.Bool("stdin", false, "把本进程的标准输入作为远程命令的标准输入")
	timeout := t.fs.Duration("timeout", defaultExecTimeout, "命令超时，上限 10m")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() == 0 {
		return t.usageError("缺少要执行的命令")
	}
	if *useStdin && *stdinFile != "" {
		return t.usageError("--stdin 和 --stdin-file 不能同时使用")
	}
	if *timeout <= 0 || *timeout > tools.MaxTimeout {
		return t.usageError("--timeout 必须大于 0 且不超过 %s", tools.MaxTimeout)
	}
	callArgs := map[string]any{"command": strings.Join(t.fs.Args(), " ")}
	if *shell != "" {
		callArgs["shell"] = *shell
	}
	if *console != "" {
		callArgs["console"] = *console
	}
	if *cwd != "" {
		callArgs["cwd"] = *cwd
	}
	if len(envs) > 0 {
		m := make(map[string]string, len(envs))
		for _, kv := range envs {
			k, v, _ := strings.Cut(kv, "=") // Set 已校验过格式
			m[k] = v
		}
		callArgs["env"] = m
	}
	if *useStdin || *stdinFile != "" {
		var data []byte
		var err error
		if *useStdin {
			data, err = io.ReadAll(stdinReader)
		} else {
			data, err = os.ReadFile(*stdinFile)
		}
		if err != nil {
			fmt.Fprintf(stderr, "读取标准输入内容失败：%v\n", err)
			return exitError
		}
		callArgs["stdin"] = string(data)
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}

	resp, interrupted, err := s.callInterruptible("proc.exec", callArgs, client.CallOptions{
		RequestID: protocol.NewRequestID(),
		TimeoutMS: int(timeout.Milliseconds()),
	})
	if interrupted {
		fmt.Fprintln(stderr, "已中断，已请求对端取消命令")
		return exitInterrupted
	}
	if err != nil {
		return s.fail(err)
	}
	var r execResult
	if err := decodeResult(resp.Result, &r); err != nil {
		return s.fail(err)
	}
	if r.Console {
		fmt.Fprintln(stderr, "（visible 模式：命令在设备桌面的控制台窗口里运行，不返回 stdout/stderr）")
	}
	if *s.jsonOut {
		if c := s.printRaw(resp.Result); c != exitOK {
			return c
		}
	} else {
		if _, err := io.WriteString(stdout, r.Stdout); err != nil {
			fmt.Fprintf(stderr, "输出失败：%v\n", err)
			return exitError
		}
		if _, err := io.WriteString(stderr, r.Stderr); err != nil {
			return exitError
		}
	}
	if r.StdoutTruncated || r.StderrTruncated {
		fmt.Fprintln(stderr, "提示：输出超过上限，已被截断")
	}
	switch {
	case r.TimedOut:
		fmt.Fprintf(stderr, "命令超时（%g 秒）\n", timeout.Seconds())
		return exitTimedOut
	case r.ExitCode < 0 || r.ExitCode > 255:
		return exitError
	}
	return r.ExitCode
}

// callInterruptible 同 call，但等待期间收到 Ctrl+C 时，用同一个 request_id
// 请求对端取消，并返回 interrupted 为 true。信号只在调用期间生效。
func (s *toolSession) callInterruptible(tool string, args any, opts client.CallOptions) (resp *protocol.CallResponse, interrupted bool, err error) {
	sigCh, stop := notifyInterrupt()
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		resp *protocol.CallResponse
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		r, e := s.call(ctx, tool, args, opts)
		done <- outcome{r, e}
	}()
	select {
	case o := <-done:
		return o.resp, false, o.err
	case <-sigCh:
		// 原 ctx 即将取消，取消请求用新的短超时 context。
		cctx, ccancel := context.WithTimeout(context.Background(), cancelTimeout)
		defer ccancel()
		if cerr := s.c.Cancel(cctx, s.device, opts.RequestID); cerr != nil {
			fmt.Fprintf(s.stderr, "请求取消失败：%v\n", cerr)
		}
		return nil, true, nil
	}
}
