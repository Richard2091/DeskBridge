package builtin

import (
	"context"
	"encoding/json"
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
