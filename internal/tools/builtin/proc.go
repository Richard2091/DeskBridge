package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// ---------- proc.exec ----------

// procExecArgs 是 proc.exec 的参数。
type procExecArgs struct {
	Command        string            `json:"command"`  // 经 shell 执行；和 argv 二选一
	Argv           []string          `json:"argv"`     // 不经 shell 直接执行；和 command 二选一
	Shell          string            `json:"shell"`    // 空为平台默认
	Cwd            string            `json:"cwd"`      // 工作目录
	Env            map[string]string `json:"env"`      // 追加或覆盖的环境变量
	Stdin          string            `json:"stdin"`    // 标准输入
	Console        string            `json:"console"`  // hidden（默认）或 visible
	MaxOutputBytes int               `json:"max_output_bytes"` // 0 用默认值
}

// execResult 是 proc.exec 的返回值。
type execResult struct {
	ExitCode        int    `json:"exit_code"` // 超时或被取消终止时为 -1
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	Console         bool   `json:"console,omitempty"` // 可见窗口模式；此时不返回 stdout/stderr
	DurationMS      int64  `json:"duration_ms"`
}

// procExecSchema 是 proc.exec 的参数描述。
const procExecSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "command": {"type": "string", "description": "命令行，交给 shell 执行；和 argv 二选一"},
    "argv": {"type": "array", "items": {"type": "string"}, "description": "程序和参数，不经过 shell 直接执行；和 command 二选一"},
    "shell": {"type": "string", "description": "执行 command 用的 shell", "enum": ["powershell", "pwsh", "cmd", "sh", "bash"]},
    "cwd": {"type": "string", "description": "工作目录，默认节点进程的当前目录"},
    "env": {"type": "object", "additionalProperties": {"type": "string"}, "description": "追加或覆盖的环境变量，如 {\"NAME\":\"value\"}"},
    "stdin": {"type": "string", "description": "标准输入；不提供时为空输入；visible 模式不支持"},
    "console": {"type": "string", "enum": ["hidden", "visible"], "default": "hidden", "description": "控制台窗口：hidden（默认）不弹窗口，输出通过返回值和实时输出事件返回；visible 在设备桌面上打开一个带标题的窗口，显示命令行和实时输出，适合下载、安装、构建等耗时较长、需要人看着的命令；visible 不返回 stdout/stderr，只返回退出码，仅 Windows 支持"},
    "max_output_bytes": {"type": "integer", "default": 1048576, "maximum": 8388608, "description": "标准输出和标准错误各自的上限，超出部分丢弃并标记 truncated"}
  }
}`

func newProcExecTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "proc.exec",
			Description: "在设备上执行一条命令或程序，返回退出码和输出。命令启动失败返回错误；非零退出不是错误，看结果里的 exit_code。Windows 上默认不弹控制台窗口；下载、安装、构建等耗时较长、需要人看着的命令可以把 console 设为 visible，在设备桌面上打开带标题的窗口显示命令行和实时输出（此时不返回 stdout/stderr，只返回退出码）。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(procExecSchema),
		},
		DefaultTimeout: 5 * time.Minute, // 命令可能较慢，默认超时比其他工具长
		Run:            runProcExec,
	}
}

func runProcExec(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a procExecArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	switch a.Console {
	case "", "hidden", "visible":
	default:
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "不支持的 console：%s（可用：hidden、visible）", a.Console)
	}
	visible := a.Console == "visible"
	if visible && a.Stdin != "" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "visible 模式不支持 stdin")
	}
	maxOutput := a.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = defaultOutputBytes
	}
	if maxOutput > maxOutputBytes {
		maxOutput = maxOutputBytes
	}
	cmd, err := buildExecCommand(&a)
	if err != nil {
		return nil, err
	}
	if a.Cwd != "" {
		cmd.Dir = a.Cwd
	}
	if len(a.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range a.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if visible {
		// 可见窗口模式：把命令交给 console 子命令，由它在新控制台窗口里运行。
		display := a.Command
		if display == "" {
			display = strings.Join(a.Argv, " ")
		}
		shell := ""
		if a.Command != "" {
			shell = a.Shell
			if shell == "" {
				shell = defaultShell
			}
		}
		cwd := cmd.Dir
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		cmd, err = visibleCommand(cmd, display, consolePrompt(shell, cwd))
		if err != nil {
			return nil, err
		}
	}
	stdout := newLimitedBuffer(maxOutput)
	stderr := newLimitedBuffer(maxOutput)
	outStream := newLineStreamer(call, "stdout")
	errStream := newLineStreamer(call, "stderr")
	if !visible {
		cmd.Stdout = io.MultiWriter(stdout, outStream)
		cmd.Stderr = io.MultiWriter(stderr, errStream)
		if a.Stdin != "" {
			cmd.Stdin = strings.NewReader(a.Stdin)
		}
	}

	kill, cleanup, err := startProcess(cmd, visible)
	if err != nil {
		return nil, mapStartError(err)
	}
	defer cleanup()

	started := time.Now()
	done := make(chan struct{})
	watcherExit := make(chan struct{})
	var killed atomic.Bool // kill 真正被触发过
	go func() {
		defer close(watcherExit)
		select {
		case <-ctx.Done():
			killed.Store(true)
			kill() // 结束整棵进程树（作业对象或进程组）
		case <-done:
		}
	}()
	// 后台子进程可能还持有输出管道：进程退出后最多再等 2 秒就放弃
	// 等待管道，不让 Wait 一直阻塞。
	cmd.WaitDelay = 2 * time.Second
	waitErr := cmd.Wait()
	outStream.Flush()
	errStream.Flush()
	close(done)
	<-watcherExit
	elapsed := time.Since(started).Milliseconds()

	// 只有 kill 真正被触发才算超时或取消；命令恰好在截止时间前后
	// 正常结束时按正常结果返回。
	switch {
	case killed.Load() && errors.Is(ctx.Err(), context.Canceled):
		return nil, ctx.Err()
	case killed.Load():
		// 超时：返回已收到的输出并标记 timed_out，不是错误。
		return &execResult{
			ExitCode:        -1,
			Stdout:          decodeOutput(stdout.b.Bytes()),
			Stderr:          decodeOutput(stderr.b.Bytes()),
			StdoutTruncated: stdout.truncated,
			StderrTruncated: stderr.truncated,
			TimedOut:        true,
			Console:         visible,
			DurationMS:      elapsed,
		}, nil
	}

	exitCode := 0
	if waitErr != nil && errors.Is(waitErr, exec.ErrWaitDelay) {
		// 命令本身正常退出，只是后台进程占着输出管道，输出已按已收到的为准。
		waitErr = nil
	}
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode() // 被信号终止时是 -1
		} else {
			return nil, protocol.Errorf(protocol.CodeInternal, "等待命令结束失败：%v", waitErr)
		}
	}
	return &execResult{
		ExitCode:        exitCode,
		Stdout:          decodeOutput(stdout.b.Bytes()),
		Stderr:          decodeOutput(stderr.b.Bytes()),
		StdoutTruncated: stdout.truncated,
		StderrTruncated: stderr.truncated,
		Console:         visible,
		DurationMS:      elapsed,
	}, nil
}

// buildExecCommand 校验参数并构造命令（shell 的调用方式见平台文件）。
func buildExecCommand(a *procExecArgs) (*exec.Cmd, error) {
	if (a.Command == "") == (len(a.Argv) == 0) {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "command 和 argv 必须恰好提供一个")
	}
	if a.Command != "" {
		shell := a.Shell
		if shell == "" {
			shell = defaultShell
		}
		if !slices.Contains(shellAllowed(), shell) {
			return nil, protocol.Errorf(protocol.CodeInvalidArgs, "不支持的 shell：%s（可用：%s）", shell, strings.Join(shellAllowed(), "、"))
		}
		return buildShellCommand(shell, a.Command)
	}
	return buildArgvCommand(a.Argv)
}

// consolePrompt 生成可见窗口里模拟的提示符：PowerShell 用 "PS 路径>"，
// 其他 shell 和直接运行程序用 "路径>"。回显的一行要像人工敲进去的命令。
func consolePrompt(shell, cwd string) string {
	if shell == "powershell" || shell == "pwsh" {
		return "PS " + cwd + "> "
	}
	return cwd + "> "
}

// buildArgvCommand 构造不经 shell 的命令。
func buildArgvCommand(argv []string) (*exec.Cmd, error) {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeNotFound, "找不到程序 %s：%v", argv[0], err)
	}
	return &exec.Cmd{Path: path, Args: argv}, nil
}

// mapStartError 把启动失败映射为协议错误。
func mapStartError(err error) error {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return protocol.Errorf(protocol.CodeNotFound, "无法启动命令：%v", err)
	}
	if errors.Is(err, os.ErrPermission) {
		return &protocol.Error{
			Code:    protocol.CodePermissionDenied,
			Message: fmt.Sprintf("操作系统拒绝了启动命令：%v", err),
			Details: map[string]any{"missing": "os_permission"},
		}
	}
	return protocol.Errorf(protocol.CodeInvalidArgs, "无法启动命令：%v", err)
}

// lineStreamer 把子进程的输出按行解码后推送给订阅者。解码要在完整的行上
// 做（GBK 等多字节编码的字符不能从中间切开）；没有换行的内容攒到
// streamChunkMax 字节或命令结束时再推送。
type lineStreamer struct {
	call   *tools.Call
	stream string
	mu     sync.Mutex
	buf    []byte
}

const streamChunkMax = 4096

func newLineStreamer(call *tools.Call, stream string) *lineStreamer {
	return &lineStreamer{call: call, stream: stream}
}

func (l *lineStreamer) Write(p []byte) (int, error) {
	if l.call.Emit == nil {
		return len(p), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	if i := bytes.LastIndexAny(l.buf, "\n\r"); i >= 0 {
		l.call.Stream(l.stream, decodeOutput(l.buf[:i+1]))
		l.buf = append(l.buf[:0], l.buf[i+1:]...)
	}
	if len(l.buf) >= streamChunkMax {
		l.call.Stream(l.stream, decodeOutput(l.buf))
		l.buf = l.buf[:0]
	}
	return len(p), nil
}

// Flush 推送剩余的不完整行。
func (l *lineStreamer) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buf) > 0 {
		l.call.Stream(l.stream, decodeOutput(l.buf))
		l.buf = l.buf[:0]
	}
}

// limitedBuffer 是有字节上限的输出缓冲：超出后丢弃多余内容并标记。
type limitedBuffer struct {
	b         bytes.Buffer
	max       int
	truncated bool
}

func newLimitedBuffer(max int) *limitedBuffer {
	return &limitedBuffer{max: max}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.b.Len(); room < len(p) {
		if room > 0 {
			l.b.Write(p[:room])
		}
		l.truncated = true
		return len(p), nil // 报告全部写入，让子进程继续，多余部分丢弃
	}
	return l.b.Write(p)
}

// ---------- proc.list ----------

// procListArgs 是 proc.list 的参数。
type procListArgs struct {
	Name       string `json:"name"` // 进程名包含该字符串，不区分大小写
	MaxEntries int    `json:"max_entries"`
}

// procEntry 是进程列表里的一项。
type procEntry struct {
	PID         int    `json:"pid"`
	PPID        int    `json:"ppid"`
	Name        string `json:"name"`
	MemoryBytes int64  `json:"memory_bytes,omitempty"`
	User        string `json:"user,omitempty"`
}

// procListResult 是 proc.list 的返回值。
type procListResult struct {
	Processes []procEntry `json:"processes"`
	Truncated bool        `json:"truncated"`
}

// procListSchema 是 proc.list 的参数描述。
const procListSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": {"type": "string", "description": "只返回进程名包含该字符串的进程，不区分大小写"},
    "max_entries": {"type": "integer", "default": 500, "maximum": 10000, "description": "返回数量的上限"}
  }
}`

func newProcListTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "proc.list",
			Description: "列出设备上正在运行的进程：PID、父进程、名称、内存等，按 PID 排序。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(procListSchema),
		},
		Run: runProcList,
	}
}

func runProcList(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a procListArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	max := a.MaxEntries
	if max <= 0 {
		max = defaultProcEntries
	}
	if max > maxProcEntries {
		max = maxProcEntries
	}
	procs, ok := listProcesses()
	if !ok {
		return nil, protocol.Errorf(protocol.CodeUnsupportedPlatform, "当前平台不支持列出进程")
	}
	if a.Name != "" {
		needle := strings.ToLower(a.Name)
		filtered := make([]procEntry, 0, len(procs))
		for _, p := range procs {
			if strings.Contains(strings.ToLower(p.Name), needle) {
				filtered = append(filtered, p)
			}
		}
		procs = filtered
	}
	slices.SortFunc(procs, func(x, y procEntry) int { return x.PID - y.PID })
	res := &procListResult{Processes: procs, Truncated: len(procs) > max}
	if res.Truncated {
		res.Processes = procs[:max]
	}
	return res, nil
}

// ---------- proc.kill ----------

// procKillArgs 是 proc.kill 的参数。
type procKillArgs struct {
	PID   int  `json:"pid"`
	Tree  bool `json:"tree"`
	Force bool `json:"force"` // Linux 上 false 发 SIGTERM、true 发 SIGKILL；Windows 上总是强制
}

// procKillResult 是 proc.kill 的返回值。
type procKillResult struct {
	Killed []int `json:"killed"`
}

// procKillSchema 是 proc.kill 的参数描述。
const procKillSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["pid"],
  "properties": {
    "pid": {"type": "integer", "description": "要结束的进程 ID"},
    "tree": {"type": "boolean", "default": false, "description": "连同它的全部子进程一起结束"},
    "force": {"type": "boolean", "default": false, "description": "Linux 上 false 发 SIGTERM、true 发 SIGKILL；Windows 上总是强制结束"}
  }
}`

func newProcKillTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "proc.kill",
			Description: "结束一个进程，可以选择连同子进程一起。不允许结束节点进程自己。返回实际结束的 PID 列表。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(procKillSchema),
		},
		Run: runProcKill,
	}
}

func runProcKill(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a procKillArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	if a.PID <= 0 {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "pid 必须是正整数")
	}
	if a.PID == os.Getpid() {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "不能结束节点进程自己（pid %d）", a.PID)
	}
	killed, err := killProcessTree(a.PID, a.Tree, a.Force)
	if err != nil { // err 非 nil 表示根进程没有被结束
		return nil, mapKillError(err)
	}
	return &procKillResult{Killed: killed}, nil
}

// plausibleParents 过滤进程快照里的父子关系。Windows 上父进程退出后子进程的
// PPID 不会更新，PID 还会被复用，所以快照里的 PPID 可能指向一个无关的新进程。
// 真正的子进程不可能比父进程更早创建：只有两者创建时间都已知、且子进程
// 创建时间不早于父进程的边才保留。取不到创建时间（如权限不足）的进程
// 一律不纳入，宁可漏杀也不误杀。created 的单位任意，只比较大小。
func plausibleParents(parents map[int]int, created map[int]uint64) map[int]int {
	out := make(map[int]int, len(parents))
	for child, parent := range parents {
		cc, ok1 := created[child]
		pc, ok2 := created[parent]
		if ok1 && ok2 && cc >= pc {
			out[child] = parent
		}
	}
	return out
}

// childrenOf 从 pid→ppid 映射里找出 pid 的全部后代，按距离从深到浅
// 返回：先杀叶子进程，避免父进程结束后子进程被重新父化而漏掉。
func childrenOf(pid int, parents map[int]int) []int {
	children := make(map[int][]int, len(parents))
	for p, pp := range parents {
		children[pp] = append(children[pp], p)
	}
	var out []int
	// visited 防止 PID 复用造成环（A 的父是 B、B 的父又是 A）时死循环。
	visited := map[int]bool{pid: true}
	queue := []int{pid}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range children[cur] {
			if visited[c] {
				continue
			}
			visited[c] = true
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	slices.Reverse(out)
	return out
}
