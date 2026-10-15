package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// jsonString 把字符串编码为 JSON 字符串字面量（带引号）。
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// itoa 是 strconv.Itoa 的短别名。
func itoa(n int) string { return strconv.Itoa(n) }

// callExec 用给定的参数表调用 proc.exec，返回结果。
func callExec(t *testing.T, args map[string]any, ctxTimeout time.Duration) (*execResult, error) {
	t.Helper()
	r := newTestRegistry(t)
	tool, ok := r.Get("proc.exec")
	if !ok {
		t.Fatal("proc.exec 没有注册")
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var cancel context.CancelFunc
	if ctxTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, ctxTimeout)
		defer cancel()
	}
	v, err := tool.Run(ctx, &tools.Call{RequestID: "t", Args: raw})
	if err != nil {
		return nil, err
	}
	res, ok := v.(*execResult)
	if !ok {
		t.Fatalf("proc.exec 结果类型不对：%T", v)
	}
	return res, nil
}

// wantExecArgs 返回各平台测试需要的参数片段。
// slowCommand 是一个至少跑几秒的命令；longRunning 是一个不会自己结束的
// 程序（供 proc.kill 测试用）。
func platformCommands() (slowCommand map[string]any, longRunning []string) {
	switch runtime.GOOS {
	case "windows":
		return map[string]any{"command": "Start-Sleep -Seconds 10", "shell": "powershell"},
			[]string{"ping", "-n", "30", "127.0.0.1"}
	default:
		return map[string]any{"command": "sleep 10"}, []string{"sleep", "30"}
	}
}

func TestProcExecEcho(t *testing.T) {
	res, err := callExec(t, map[string]any{"command": "echo hello"}, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit_code = %d，期望 0", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "hello" {
		t.Errorf("stdout = %q，期望 hello", res.Stdout)
	}
	if res.TimedOut || res.StdoutTruncated || res.StderrTruncated {
		t.Errorf("timed_out/truncated 不应为 true：%+v", res)
	}
	if res.DurationMS < 0 {
		t.Errorf("duration_ms = %d", res.DurationMS)
	}
}

func TestProcExecExitCode(t *testing.T) {
	res, err := callExec(t, map[string]any{"command": "exit 3"}, 0)
	if err != nil {
		t.Fatalf("非零退出不应返回错误: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit_code = %d，期望 3", res.ExitCode)
	}
}

func TestProcExecArgv(t *testing.T) {
	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/c", "echo", "hi"}
	} else {
		argv = []string{"sh", "-c", "echo hi"}
	}
	res, err := callExec(t, map[string]any{"argv": argv}, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "hi" {
		t.Errorf("stdout = %q，期望 hi", res.Stdout)
	}
}

func TestProcExecStdin(t *testing.T) {
	args := map[string]any{"stdin": "流数据"}
	if runtime.GOOS == "windows" {
		args["command"] = "[Console]::In.ReadToEnd()"
		args["shell"] = "powershell"
	} else {
		args["command"] = "cat"
	}
	res, err := callExec(t, args, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "流数据" {
		t.Errorf("stdout = %q，期望 流数据", res.Stdout)
	}
}

func TestProcExecEnv(t *testing.T) {
	args := map[string]any{"env": map[string]string{"DB_TEST_VAR": "值1"}}
	if runtime.GOOS == "windows" {
		args["command"] = "echo %DB_TEST_VAR%"
		args["shell"] = "cmd"
	} else {
		args["command"] = "echo $DB_TEST_VAR"
	}
	res, err := callExec(t, args, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "值1" {
		t.Errorf("stdout = %q，期望 值1", res.Stdout)
	}
}

func TestProcExecCwd(t *testing.T) {
	dir := t.TempDir()
	args := map[string]any{"cwd": dir}
	if runtime.GOOS == "windows" {
		args["command"] = "cd"
		args["shell"] = "cmd"
	} else {
		args["command"] = "pwd"
	}
	res, err := callExec(t, args, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	got := strings.TrimSpace(res.Stdout)
	if runtime.GOOS == "windows" {
		got = strings.TrimSpace(strings.SplitN(got, "\n", 2)[0])
		if !strings.EqualFold(got, dir) {
			t.Errorf("cwd 输出 = %q，期望 %q", got, dir)
		}
		return
	}
	if got != dir {
		t.Errorf("cwd 输出 = %q，期望 %q", got, dir)
	}
}

func TestProcExecCwdMissing(t *testing.T) {
	_, err := callExec(t, map[string]any{"command": "echo hi", "cwd": filepath.Join(t.TempDir(), "不存在")}, 0)
	if err == nil {
		t.Fatal("cwd 不存在应报错")
	}
	var perr *protocol.Error
	if !asProtoError(err, &perr) {
		t.Fatalf("期望协议错误：%v", err)
	}
	if perr.Code != protocol.CodeNotFound && perr.Code != protocol.CodeInvalidArgs {
		t.Errorf("错误码 = %s，期望 not_found 或 invalid_args", perr.Code)
	}
}

func TestProcExecNotFound(t *testing.T) {
	_, err := callExec(t, map[string]any{"argv": []string{"no-such-db-program-xyz"}}, 0)
	expectProtoError(t, err, protocol.CodeNotFound)
}

func TestProcExecInvalidArgs(t *testing.T) {
	// command 和 argv 同时提供。
	_, err := callExec(t, map[string]any{"command": "echo", "argv": []string{"echo"}}, 0)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
	// 都不提供。
	_, err = callExec(t, map[string]any{}, 0)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
	// 不支持的 shell。
	_, err = callExec(t, map[string]any{"command": "echo", "shell": "fish"}, 0)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
	// 未知字段。
	_, err = callExec(t, map[string]any{"command": "echo", "nope": 1}, 0)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestProcExecTimeout(t *testing.T) {
	slow, _ := platformCommands()
	start := time.Now()
	res, err := callExec(t, slow, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("超时应返回结果而不是错误: %v", err)
	}
	if !res.TimedOut {
		t.Errorf("timed_out = false，期望 true")
	}
	if res.ExitCode != -1 {
		t.Errorf("exit_code = %d，期望 -1", res.ExitCode)
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Errorf("过快返回（%v），像是没有等到超时", elapsed)
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Errorf("超时后 %v 才返回，进程树可能没被结束", elapsed)
	}
}

func TestProcExecOutputTruncated(t *testing.T) {
	// 生成约 300 字节的输出，max_output_bytes=20 应标记截断。
	args := map[string]any{"max_output_bytes": 20}
	if runtime.GOOS == "windows" {
		args["command"] = "1..100"
		args["shell"] = "powershell"
	} else {
		args["command"] = "seq 1 100"
	}
	res, err := callExec(t, args, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if !res.StdoutTruncated {
		t.Error("stdout_truncated 应为 true")
	}
	if len(res.Stdout) > 20 {
		t.Errorf("stdout 长度 = %d，超过 20", len(res.Stdout))
	}
}

func TestProcList(t *testing.T) {
	r := newTestRegistry(t)
	tool, _ := r.Get("proc.list")

	// 用进程名过滤：go test 的可执行文件名里能找到自己。
	selfName := filepath.Base(os.Args[0])
	raw, _ := json.Marshal(map[string]any{"name": strings.TrimSuffix(selfName, ".exe")})
	v, err := tool.Run(context.Background(), &tools.Call{RequestID: "t", Args: raw})
	if err != nil {
		t.Fatalf("proc.list 失败: %v", err)
	}
	res := v.(*procListResult)
	if len(res.Processes) == 0 {
		t.Fatal("进程列表为空")
	}
	foundSelf := false
	last := -1
	for _, p := range res.Processes {
		if p.PID <= last {
			t.Errorf("进程没有按 PID 排序：%d 出现在 %d 之后", p.PID, last)
		}
		last = p.PID
		if p.PID == os.Getpid() {
			foundSelf = true
			if p.Name == "" || p.PPID <= 0 {
				t.Errorf("自己的进程信息不完整：%+v", p)
			}
		}
	}
	if !foundSelf {
		t.Errorf("按 %q 过滤后没有找到自己的 PID %d", selfName, os.Getpid())
	}

	// max_entries 截断。
	raw, _ = json.Marshal(map[string]any{"max_entries": 1})
	v, err = tool.Run(context.Background(), &tools.Call{RequestID: "t", Args: raw})
	if err != nil {
		t.Fatalf("proc.list 失败: %v", err)
	}
	res = v.(*procListResult)
	if len(res.Processes) != 1 || !res.Truncated {
		t.Errorf("截断结果不对：n=%d truncated=%v", len(res.Processes), res.Truncated)
	}
}

func TestProcKill(t *testing.T) {
	_, longRunning := platformCommands()
	cmd := exec.Command(longRunning[0], longRunning[1:]...)
	cmd.Stdout = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试进程失败: %v", err)
	}
	pid := cmd.Process.Pid

	r := newTestRegistry(t)
	tool, _ := r.Get("proc.kill")

	// 不允许杀自己。
	raw, _ := json.Marshal(map[string]any{"pid": os.Getpid()})
	if _, err := tool.Run(context.Background(), &tools.Call{Args: raw}); err == nil {
		t.Error("结束节点自己应报错")
	} else {
		var perr *protocol.Error
		if !asProtoError(err, &perr) || perr.Code != protocol.CodeInvalidArgs {
			t.Errorf("杀自己应返回 invalid_args：%v", err)
		}
	}

	// 不存在的 PID。
	raw, _ = json.Marshal(map[string]any{"pid": 2147470000, "force": true})
	if _, err := tool.Run(context.Background(), &tools.Call{Args: raw}); err == nil {
		t.Error("结束不存在的进程应报错")
	} else {
		var perr *protocol.Error
		if !asProtoError(err, &perr) || perr.Code != protocol.CodeNotFound {
			t.Errorf("杀不存在的进程应返回 not_found：%v", err)
		}
	}

	// 正常结束。
	raw, _ = json.Marshal(map[string]any{"pid": pid, "force": true})
	v, err := tool.Run(context.Background(), &tools.Call{Args: raw})
	if err != nil {
		t.Fatalf("proc.kill 失败: %v", err)
	}
	res := v.(*procKillResult)
	if len(res.Killed) != 1 || res.Killed[0] != pid {
		t.Errorf("killed = %v，期望 [%d]", res.Killed, pid)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("进程在 proc.kill 后 5 秒还没有退出")
	}
}

// TestProcExecStderr 覆盖错误流的传递和截断标记。
func TestProcExecStderr(t *testing.T) {
	args := map[string]any{"max_output_bytes": 5}
	if runtime.GOOS == "windows" {
		args["command"] = "Write-Error '中文错误'"
		args["shell"] = "powershell"
	} else {
		args["command"] = "echo 中文错误信息输出到错误流 1>&2"
	}
	res, err := callExec(t, args, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if res.Stderr == "" {
		t.Fatal("stderr 为空")
	}
	if !utf8.ValidString(res.Stderr) {
		t.Fatalf("stderr 不是合法 UTF-8：%q", res.Stderr)
	}
	if !res.StderrTruncated {
		t.Errorf("stderr_truncated 应为 true（%q）", res.Stderr)
	}
}

// listPIDsByName 用 proc.list 按名字找进程，返回 PID 集合。
func listPIDsByName(t *testing.T, name string) map[int]bool {
	t.Helper()
	r := newTestRegistry(t)
	v, err := callByName(t, r, "proc.list", `{"name":`+jsonString(name)+`,"max_entries":10000}`)
	if err != nil {
		t.Fatalf("proc.list 失败: %v", err)
	}
	out := map[int]bool{}
	for _, p := range v.(*procListResult).Processes {
		out[p.PID] = true
	}
	return out
}

// newPIDs 返回 now 里有而 before 里没有的 PID。
func newPIDs(before, now map[int]bool) []int {
	var out []int
	for pid := range now {
		if !before[pid] {
			out = append(out, pid)
		}
	}
	return out
}

// uniqueProgram 把系统程序 src（如 ping、sleep）复制到临时目录，起一个本次
// 测试独有的名字，返回路径和进程名。后台进程测试按进程名统计和清理进程，
// 用系统程序原名时，同机同时运行的其他测试（或其他程序）的同名进程会被
// 算进来，清理时还会被误杀。
func uniqueProgram(t *testing.T, src string) (path, name string) {
	t.Helper()
	orig, err := exec.LookPath(src)
	if err != nil {
		t.Fatalf("找不到 %s: %v", src, err)
	}
	data, err := os.ReadFile(orig)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", orig, err)
	}
	// Linux 的进程名（comm）最多 15 个字符。
	name = fmt.Sprintf("dbt%08x", rand.Uint32())
	path = filepath.Join(t.TempDir(), name+filepath.Ext(orig))
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatalf("复制 %s 失败: %v", orig, err)
	}
	return path, name
}

// killPIDs 清理测试启动的进程。
func killPIDs(t *testing.T, pids []int) {
	for _, pid := range pids {
		raw, _ := json.Marshal(map[string]any{"pid": pid, "force": true})
		tool, _ := newTestRegistry(t).Get("proc.kill")
		_, _ = tool.Run(context.Background(), &tools.Call{Args: raw})
	}
}

// runBackgroundKept 验证命令正常结束后，它启动的后台进程还在。
func runBackgroundKept(t *testing.T, args map[string]any, procName string) {
	t.Helper()
	before := listPIDsByName(t, procName)
	start := time.Now()
	res, err := callExec(t, args, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("耗时 %v，应在 5 秒内返回（后台进程不应拖住命令）", elapsed)
	}
	if res.ExitCode != 0 || res.TimedOut {
		t.Errorf("exit_code=%d timed_out=%v，期望 0/false（stderr=%q）", res.ExitCode, res.TimedOut, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "started") {
		t.Errorf("stdout = %q，应包含 started", res.Stdout)
	}
	created := newPIDs(before, listPIDsByName(t, procName))
	defer killPIDs(t, created)
	if len(created) == 0 {
		t.Errorf("命令正常结束后，后台的 %s 进程被结束了", procName)
	}
}

// runTimeoutKillsChildren 验证超时返回 timed_out，且命令启动的子进程被结束。
func runTimeoutKillsChildren(t *testing.T, command, shell, procName string) {
	t.Helper()
	before := listPIDsByName(t, procName)
	start := time.Now()
	res, err := callExec(t, map[string]any{"command": command, "shell": shell}, time.Second)
	if err != nil {
		t.Fatalf("超时应返回结果而不是错误: %v", err)
	}
	if !res.TimedOut {
		t.Errorf("timed_out = false，期望 true")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("超时后 %v 才返回", elapsed)
	}
	created := newPIDs(before, listPIDsByName(t, procName))
	defer killPIDs(t, created)
	deadline := time.Now().Add(5 * time.Second)
	for len(created) > 0 && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		created = newPIDs(before, listPIDsByName(t, procName))
	}
	if len(created) > 0 {
		t.Errorf("超时后子进程 %v 仍在运行", created)
	}
}

func TestChildrenOfCycle(t *testing.T) {
	// 1 -> 2 -> 3 -> 2（PID 复用成环），不能死循环，也不能重复。
	parents := map[int]int{2: 1, 3: 2, 1: 3}
	done := make(chan []int, 1)
	go func() { done <- childrenOf(1, parents) }()
	select {
	case got := <-done:
		seen := map[int]bool{}
		for _, p := range got {
			if seen[p] || p == 1 {
				t.Errorf("结果有重复或包含起点：%v", got)
			}
			seen[p] = true
		}
		if len(got) != 2 {
			t.Errorf("后代 = %v，期望 2 个", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("childrenOf 遇到环没有返回")
	}
}

func TestTruncateAtRuneBoundary(t *testing.T) {
	cases := []struct {
		in   []byte
		want int
	}{
		{[]byte("中文"), 6},
		{[]byte("中文测")[:7], 6},
		{[]byte("中文测")[:8], 6},
		{[]byte("a中")[:2], 1},
		{[]byte{0xFF, 'a'}, 2}, // 中间的非法字节不回退
		{nil, 0},
	}
	for _, c := range cases {
		if got := truncateAtRuneBoundary(c.in); got != c.want {
			t.Errorf("truncateAtRuneBoundary(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

func TestDecodeOutputTruncatedUTF8(t *testing.T) {
	// 截断在多字节字符中间时，仍按 UTF-8 正确解码（半个字符丢弃）。
	b := []byte("中文测试")[:10]
	if got := decodeOutput(b); got != "中文测" {
		t.Errorf("decodeOutput = %q，期望 中文测", got)
	}
}

func TestProcKillTreeSkipsSelf(t *testing.T) {
	// tree=true 时结束节点自己同样返回 invalid_args。
	tool, _ := newTestRegistry(t).Get("proc.kill")
	raw, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "tree": true})
	_, err := tool.Run(context.Background(), &tools.Call{Args: raw})
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestDefaultTimeoutOf(t *testing.T) {
	if d, ok := DefaultTimeoutOf("proc.exec"); !ok || d != 5*time.Minute {
		t.Fatalf("proc.exec = %v, %v", d, ok)
	}
	if d, ok := DefaultTimeoutOf("fs.stat"); !ok || d != tools.DefaultTimeout {
		t.Fatalf("fs.stat = %v, %v", d, ok)
	}
	if _, ok := DefaultTimeoutOf("no.such"); ok {
		t.Fatal("未知工具不应找到")
	}
}
