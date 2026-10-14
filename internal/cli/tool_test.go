package cli

// 工具类子命令的测试：用 meshtest.Fake 起一个真的节点，注册假工具，
// 经 DESKBRIDGE_HOME 下的 unix socket 调用。socket 放在 os.MkdirTemp 建的
// 短路径目录里（Windows 上 t.TempDir() 可能超过 AF_UNIX 的 108 字符限制）。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/deviceid"
	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/mesh/meshtest"
	"github.com/Richard2091/DeskBridge/internal/node"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// fakeEnv 是测试节点和它的假工具共享的状态。
type fakeEnv struct {
	mu        sync.Mutex
	files     map[string][]byte // fs.read、fs.write 操作的内存文件系统
	writes    int               // fs.write 调用次数
	execArgs  map[string]any    // 最近一次 proc.exec 的参数
	hangStart chan struct{}     // "hang" 命令开始执行时关闭
	hangDone  chan struct{}     // "hang" 命令的 ctx 被取消时关闭
	fake      *meshtest.Fake
}

// readChunkCap 是假 fs.read 每块最多返回的字节数，故意比请求的小，
// 迫使 read 子命令多次循环。
const readChunkCap = 3

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "db")
	if err != nil {
		t.Fatalf("创建临时目录失败：%v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func (e *fakeEnv) tool(name string, run func(ctx context.Context, call *tools.Call) (any, error)) *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{Name: name, Description: "测试用 " + name, Source: protocol.SourceBuiltin},
		Run:  run,
	}
}

// registry 返回假工具集。
func (e *fakeEnv) registry(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	for _, tool := range []*tools.Tool{
		e.tool("fs.read", e.fsRead),
		e.tool("fs.write", e.fsWrite),
		e.tool("proc.exec", e.procExec),
		e.tool("fs.list", func(context.Context, *tools.Call) (any, error) {
			return map[string]any{"path": "/d", "truncated": true, "entries": []map[string]any{
				{"name": "sub", "type": "dir", "size": 0, "mtime": "2026-10-14T21:05:00+08:00"},
				{"name": "a.txt", "type": "file", "size": 12, "mtime": "2026-10-14T21:05:00+08:00"},
			}}, nil
		}),
		e.tool("fs.stat", func(context.Context, *tools.Call) (any, error) {
			return map[string]any{"path": "/d/a.txt", "type": "file", "size": 12, "mtime": "2026-10-14T21:05:00+08:00", "mode": "-rw-r--r--"}, nil
		}),
		e.tool("sys.info", func(context.Context, *tools.Call) (any, error) {
			return map[string]any{"name": "win-pc", "os": "windows", "num_cpu": 8, "session": map[string]any{"id": 1, "interactive": true}}, nil
		}),
		e.tool("proc.list", func(context.Context, *tools.Call) (any, error) {
			return map[string]any{"processes": []map[string]any{{"pid": 4, "ppid": 0, "name": "System", "memory_bytes": 2097152}}}, nil
		}),
		e.tool("proc.kill", func(context.Context, *tools.Call) (any, error) {
			return map[string]any{"killed": []int{42, 43}}, nil
		}),
	} {
		if err := reg.Register(tool); err != nil {
			t.Fatalf("注册工具失败：%v", err)
		}
	}
	return reg
}

func (e *fakeEnv) fsRead(_ context.Context, call *tools.Call) (any, error) {
	var a struct {
		Path     string `json:"path"`
		Offset   int64  `json:"offset"`
		MaxBytes int64  `json:"max_bytes"`
		Encoding string `json:"encoding"`
	}
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	if a.Path == "stuck" { // 异常对端：既没有数据也不报告结尾
		return map[string]any{"path": a.Path, "content": "", "encoding": "base64", "size": 10, "offset": a.Offset, "bytes_read": 0, "eof": false}, nil
	}
	e.mu.Lock()
	data, ok := e.files[a.Path]
	e.mu.Unlock()
	if !ok {
		return nil, protocol.Errorf(protocol.CodeNotFound, "文件不存在：%s", a.Path)
	}
	end := a.Offset + min(a.MaxBytes, readChunkCap)
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	chunk := data[a.Offset:end]
	content, enc := base64.StdEncoding.EncodeToString(chunk), "base64"
	if a.Encoding == "auto" {
		content, enc = string(chunk), "utf-8"
	}
	return map[string]any{
		"path": a.Path, "content": content, "encoding": enc, "size": len(data),
		"offset": a.Offset, "bytes_read": len(chunk), "eof": end == int64(len(data)),
	}, nil
}

func (e *fakeEnv) fsWrite(_ context.Context, call *tools.Call) (any, error) {
	var a struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Append   bool   `json:"append"`
	}
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	data := []byte(a.Content)
	if a.Encoding == "base64" {
		var err error
		if data, err = base64.StdEncoding.DecodeString(a.Content); err != nil {
			return nil, protocol.Errorf(protocol.CodeInvalidArgs, "base64 不合法")
		}
	}
	written := len(data)
	e.mu.Lock()
	defer e.mu.Unlock()
	old, existed := e.files[a.Path]
	if a.Append {
		data = append(append([]byte{}, old...), data...)
	}
	e.files[a.Path] = data
	e.writes++
	return map[string]any{"path": "/abs/" + a.Path, "bytes_written": written, "created": !existed}, nil
}

func (e *fakeEnv) procExec(ctx context.Context, call *tools.Call) (any, error) {
	var a map[string]any
	if err := json.Unmarshal(call.Args, &a); err != nil {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "参数不合法")
	}
	e.mu.Lock()
	e.execArgs = a
	e.mu.Unlock()
	switch a["command"] {
	case "fail":
		return map[string]any{"exit_code": 3, "stdout": "你好\n", "stderr": "错误输出\n", "duration_ms": 5}, nil
	case "slow":
		return map[string]any{"exit_code": -1, "stdout": "部分输出", "stderr": "", "timed_out": true, "duration_ms": 2000}, nil
	case "big":
		return map[string]any{"exit_code": 0, "stdout": "x", "stdout_truncated": true}, nil
	case "huge":
		return map[string]any{"exit_code": 1000, "stdout": ""}, nil
	case "hang":
		close(e.hangStart)
		<-ctx.Done()
		close(e.hangDone)
		return nil, ctx.Err()
	}
	return map[string]any{"exit_code": 0, "stdout": "ok", "stderr": ""}, nil
}

// startTestNode 启动带假工具的节点，设置 DESKBRIDGE_HOME 指向它的 socket 目录。
func startTestNode(t *testing.T) *fakeEnv {
	t.Helper()
	home := shortTempDir(t)
	t.Setenv("DESKBRIDGE_HOME", home)
	t.Setenv(envDevice, "")
	t.Setenv(envInitiator, "")
	env := &fakeEnv{
		files:     map[string][]byte{},
		hangStart: make(chan struct{}),
		hangDone:  make(chan struct{}),
	}
	fake := meshtest.NewFake(mesh.KindTailscale, "100.64.0.1")
	fake.SetStatus(mesh.Status{
		Kind: mesh.KindTailscale, State: mesh.StateConnected, Account: "alice@example.com",
		Self: &mesh.Peer{ID: "self-1", Name: "win-pc", HostName: "WIN-PC", Addrs: []string{"100.64.0.1"},
			OS: "windows", Mine: true, Online: true, Client: mesh.ClientDeskBridge},
	})
	fake.SetDefaultIdentity(mesh.Identity{Kind: mesh.KindTailscale, Owner: "me@example.com", Device: "win-pc", Mine: true})
	fake.SetPeers([]mesh.Peer{{
		ID: "p1", Name: "desk", Addrs: []string{"100.64.0.1"}, Mine: true, Online: true,
		Path: mesh.PathRelay, Relay: "hkg", LatencyMS: 231, Client: mesh.ClientDeskBridge, OS: "linux",
	}, {
		ID: "p2", Name: "phone", Addrs: []string{"100.64.0.9"}, Mine: true, Online: true,
		Client: mesh.ClientTailscale, OS: "android", Path: mesh.PathRelay, Relay: "hkg", LatencyMS: 231,
	}, {
		ID: "p3", Name: "tsnet-app", Addrs: []string{"100.64.0.10"}, Mine: true, Online: false,
		Client: mesh.ClientTSNet,
	}})
	env.fake = fake
	dev, err := deviceid.Load(&deviceid.FileStore{Path: filepath.Join(home, "device.key")})
	if err != nil {
		t.Fatalf("加载设备 ID 失败：%v", err)
	}
	n := node.New(node.Config{
		Channels: []mesh.Channel{fake},
		Device:   dev,
		Tools:    env.registry(t),
		Name:     "win-pc",
		Socket:   filepath.Join(home, "node.sock"),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("节点没有在 5 秒内退出")
		}
	})
	for i := 0; i < 100; i++ {
		_, serr := os.Stat(filepath.Join(home, "node.sock"))
		if serr == nil && fake.RealAddr("100.64.0.1:7465") != "" {
			return env
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("节点没有在超时内启动")
	return nil
}

// runCLI 运行子命令，返回退出码、标准输出和标准错误。
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func setStdin(t *testing.T, content string) {
	t.Helper()
	old := stdinReader
	stdinReader = strings.NewReader(content)
	t.Cleanup(func() { stdinReader = old })
}

func TestExecOutputAndExitCode(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "exec", "-d", "self", "fail")
	if code != 3 {
		t.Errorf("退出码 = %d，期望 3", code)
	}
	if stdout != "你好\n" {
		t.Errorf("标准输出 = %q", stdout)
	}
	if stderr != "错误输出\n" {
		t.Errorf("标准错误 = %q", stderr)
	}
}

func TestExecTimedOut(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "exec", "-d", "self", "--timeout", "2s", "slow")
	if code != exitTimedOut {
		t.Errorf("退出码 = %d，期望 124", code)
	}
	if stdout != "部分输出" || !strings.Contains(stderr, "命令超时（2 秒）") {
		t.Errorf("标准输出 = %q，标准错误 = %q", stdout, stderr)
	}
}

func TestExecTruncatedAndOutOfRangeExitCode(t *testing.T) {
	startTestNode(t)
	code, _, stderr := runCLI(t, "exec", "-d", "self", "big")
	if code != 0 || !strings.Contains(stderr, "截断") {
		t.Errorf("big：退出码 = %d，标准错误 = %q", code, stderr)
	}
	if code, _, _ = runCLI(t, "exec", "-d", "self", "huge"); code != exitError {
		t.Errorf("huge：退出码 = %d，期望 1", code)
	}
}

func TestExecPassesArguments(t *testing.T) {
	env := startTestNode(t)
	stdinFile := filepath.Join(shortTempDir(t), "in.txt")
	if err := os.WriteFile(stdinFile, []byte("输入内容"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCLI(t, "exec", "-d", "self", "--shell", "cmd", "--cwd", "D:/x",
		"--env", "A=1", "--env", "B=x=y", "--stdin-file", stdinFile, "--", "echo", "-n", "hi")
	if code != 0 || stdout != "ok" {
		t.Fatalf("退出码 = %d，标准输出 = %q", code, stdout)
	}
	env.mu.Lock()
	defer env.mu.Unlock()
	a := env.execArgs
	if a["command"] != "echo -n hi" || a["shell"] != "cmd" || a["cwd"] != "D:/x" || a["stdin"] != "输入内容" {
		t.Errorf("参数 = %v", a)
	}
	envMap, _ := a["env"].(map[string]any)
	if envMap["A"] != "1" || envMap["B"] != "x=y" {
		t.Errorf("env = %v", a["env"])
	}
}

func TestExecStdin(t *testing.T) {
	env := startTestNode(t)
	setStdin(t, "来自管道")
	if code, _, _ := runCLI(t, "exec", "-d", "self", "--stdin", "cat"); code != 0 {
		t.Fatalf("退出码 = %d", code)
	}
	env.mu.Lock()
	defer env.mu.Unlock()
	if env.execArgs["stdin"] != "来自管道" {
		t.Errorf("stdin = %v", env.execArgs["stdin"])
	}
}

func TestExecJSON(t *testing.T) {
	startTestNode(t)
	code, stdout, _ := runCLI(t, "exec", "-d", "self", "--json", "fail")
	if code != 3 {
		t.Errorf("退出码 = %d，期望 3", code)
	}
	var r execResult
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("标准输出不是单个 JSON 值：%v（%q）", err, stdout)
	}
	if r.ExitCode != 3 || r.Stdout != "你好\n" {
		t.Errorf("result = %+v", r)
	}
}

func TestExecUsageErrors(t *testing.T) {
	startTestNode(t)
	for name, args := range map[string][]string{
		"缺命令":      {"exec", "-d", "self"},
		"超时超过上限":   {"exec", "-d", "self", "--timeout", "11m", "x"},
		"超时为零":     {"exec", "-d", "self", "--timeout", "0s", "x"},
		"stdin 冲突": {"exec", "-d", "self", "--stdin", "--stdin-file", "f", "x"},
		"env 格式":   {"exec", "-d", "self", "--env", "AB", "x"},
	} {
		if code, _, _ := runCLI(t, args...); code != exitUsage {
			t.Errorf("%s：退出码 = %d，期望 2", name, code)
		}
	}
}

func TestExecInterrupt(t *testing.T) {
	env := startTestNode(t)
	sig := make(chan os.Signal, 1)
	old := notifyInterrupt
	notifyInterrupt = func() (<-chan os.Signal, func()) { return sig, func() {} }
	t.Cleanup(func() { notifyInterrupt = old })

	type result struct {
		code   int
		stderr string
	}
	done := make(chan result, 1)
	go func() {
		code, _, stderr := runCLI(t, "exec", "-d", "self", "hang")
		done <- result{code, stderr}
	}()
	select {
	case <-env.hangStart:
	case <-time.After(5 * time.Second):
		t.Fatal("远程命令没有开始执行")
	}
	sig <- os.Interrupt
	select {
	case r := <-done:
		if r.code != exitInterrupted {
			t.Errorf("退出码 = %d，期望 130（%s）", r.code, r.stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Ctrl+C 后没有退出")
	}
	select {
	case <-env.hangDone:
	case <-time.After(5 * time.Second):
		t.Error("对端没有取消命令")
	}
}

func TestReadChunksToStdout(t *testing.T) {
	env := startTestNode(t)
	want := []byte("你好，世界\x00\xff二进制")
	env.files["f.bin"] = want
	code, stdout, stderr := runCLI(t, "read", "-d", "self", "f.bin")
	if code != 0 {
		t.Fatalf("退出码 = %d（%s）", code, stderr)
	}
	if stdout != string(want) {
		t.Errorf("标准输出 = %q，期望 %q", stdout, want)
	}
}

func TestReadToFileAndEmptyFile(t *testing.T) {
	env := startTestNode(t)
	env.files["a"] = []byte("0123456789")
	env.files["empty"] = nil
	dir := shortTempDir(t)
	out := filepath.Join(dir, "a.out")
	if code, stdout, _ := runCLI(t, "read", "-d", "self", "--out", out, "a"); code != 0 || stdout != "" {
		t.Fatalf("退出码 = %d，标准输出 = %q", code, stdout)
	}
	if got, _ := os.ReadFile(out); string(got) != "0123456789" {
		t.Errorf("文件内容 = %q", got)
	}
	out2 := filepath.Join(dir, "e.out")
	if code, _, _ := runCLI(t, "read", "-d", "self", "--out", out2, "empty"); code != 0 {
		t.Fatalf("空文件退出码 = %d", code)
	}
	if st, err := os.Stat(out2); err != nil || st.Size() != 0 {
		t.Errorf("空文件应创建出空的本地文件：%v", err)
	}
	// 读取失败时不留下本地文件。
	out3 := filepath.Join(dir, "none.out")
	if code, _, stderr := runCLI(t, "read", "-d", "self", "--out", out3, "missing"); code != exitError || !strings.Contains(stderr, "错误（not_found）") {
		t.Errorf("退出码 = %d，标准错误 = %q", code, stderr)
	}
	if _, err := os.Stat(out3); err == nil {
		t.Error("读取失败不应创建本地文件")
	}
}

func TestReadStopsWithoutProgress(t *testing.T) {
	startTestNode(t)
	done := make(chan int, 1)
	go func() {
		code, _, _ := runCLI(t, "read", "-d", "self", "stuck")
		done <- code
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("退出码 = %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("bytes_read 为 0 时 read 没有停止")
	}
}

func TestReadJSONSingleChunk(t *testing.T) {
	env := startTestNode(t)
	env.files["f"] = []byte("abcdef")
	code, stdout, _ := runCLI(t, "read", "-d", "self", "--json", "f")
	if code != 0 {
		t.Fatalf("退出码 = %d", code)
	}
	var r fsReadResult
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("标准输出不是单个 JSON 值：%v", err)
	}
	if r.Content != "abc" || r.EOF || r.Encoding != "utf-8" {
		t.Errorf("result = %+v（--json 只读一块，encoding auto）", r)
	}
}

func TestWriteText(t *testing.T) {
	env := startTestNode(t)
	code, stdout, _ := runCLI(t, "write", "-d", "self", "w.txt", "你好")
	if code != 0 || stdout != "已写入 6 字节到 /abs/w.txt\n" {
		t.Errorf("退出码 = %d，标准输出 = %q", code, stdout)
	}
	if code, _, _ = runCLI(t, "write", "-d", "self", "--append", "w.txt", "!"); code != 0 {
		t.Fatalf("追加退出码 = %d", code)
	}
	if got := string(env.files["w.txt"]); got != "你好!" {
		t.Errorf("文件内容 = %q", got)
	}
}

func TestWriteFromFileInChunks(t *testing.T) {
	env := startTestNode(t)
	data := bytes.Repeat([]byte{0xab, 0x01, 0x02}, (fileChunk+10)/3+1) // 略多于一块，含非 UTF-8 字节
	src := filepath.Join(shortTempDir(t), "src.bin")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	env.files["dst"] = []byte("旧内容应被覆盖")
	code, stdout, stderr := runCLI(t, "write", "-d", "self", "--from", src, "dst")
	if code != 0 {
		t.Fatalf("退出码 = %d（%s）", code, stderr)
	}
	if !bytes.Equal(env.files["dst"], data) {
		t.Errorf("远程文件内容与源文件不一致（%d 字节 vs %d 字节）", len(env.files["dst"]), len(data))
	}
	if env.writes != 2 {
		t.Errorf("fs.write 调用 %d 次，期望 2 次", env.writes)
	}
	if want := "已写入 " + strconv.Itoa(len(data)) + " 字节到 /abs/dst\n"; stdout != want {
		t.Errorf("标准输出 = %q，期望 %q", stdout, want)
	}
}

func TestWriteFromExactChunkAndAppend(t *testing.T) {
	env := startTestNode(t)
	data := bytes.Repeat([]byte("a"), fileChunk)
	setStdin(t, string(data))
	env.files["dst"] = []byte("head-")
	if code, _, stderr := runCLI(t, "write", "-d", "self", "--from", "-", "--append", "dst"); code != 0 {
		t.Fatalf("退出码 = %d（%s）", code, stderr)
	}
	if !bytes.Equal(env.files["dst"], append([]byte("head-"), data...)) {
		t.Error("追加后的内容不对")
	}
	if env.writes != 1 {
		t.Errorf("恰好一块时 fs.write 应调用 1 次，实际 %d 次", env.writes)
	}
}

func TestWriteFromEmptyCreatesFile(t *testing.T) {
	env := startTestNode(t)
	setStdin(t, "")
	if code, _, _ := runCLI(t, "write", "-d", "self", "--from", "-", "empty"); code != 0 {
		t.Fatalf("退出码 = %d", code)
	}
	if got, ok := env.files["empty"]; !ok || len(got) != 0 {
		t.Errorf("应创建空文件：%v %v", got, ok)
	}
}

func TestWriteJSONAndUsage(t *testing.T) {
	startTestNode(t)
	code, stdout, _ := runCLI(t, "write", "-d", "self", "--json", "j.txt", "abc")
	var r fsWriteResult
	if code != 0 || json.Unmarshal([]byte(stdout), &r) != nil || r.BytesWritten != 3 || !r.Created {
		t.Errorf("退出码 = %d，标准输出 = %q", code, stdout)
	}
	for name, args := range map[string][]string{
		"缺内容":        {"write", "-d", "self", "p"},
		"内容和 --from": {"write", "-d", "self", "--from", "f", "p", "c"},
		"缺路径":        {"write", "-d", "self"},
	} {
		if code, _, _ := runCLI(t, args...); code != exitUsage {
			t.Errorf("%s：退出码 = %d，期望 2", name, code)
		}
	}
}

func TestListStatInfoPsKill(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "ls", "-d", "self", "-r", "--max", "5", "/d")
	if code != 0 || !strings.Contains(stdout, "sub/") || !strings.Contains(stdout, "a.txt") || !strings.Contains(stdout, "d  ") {
		t.Errorf("ls：退出码 = %d，标准输出 = %q", code, stdout)
	}
	if !strings.Contains(stderr, "截断") {
		t.Errorf("ls 截断提示 = %q", stderr)
	}
	if code, stdout, _ = runCLI(t, "stat", "-d", "self", "/d/a.txt"); code != 0 || !strings.Contains(stdout, "类型：file") || !strings.Contains(stdout, "权限：-rw-r--r--") {
		t.Errorf("stat：%d %q", code, stdout)
	}
	if code, stdout, _ = runCLI(t, "info", "-d", "self"); code != 0 || !strings.HasPrefix(stdout, "name：win-pc\nos：windows\n") || !strings.Contains(stdout, `session：{"id":1,"interactive":true}`) {
		t.Errorf("info：%d %q", code, stdout)
	}
	if code, stdout, _ = runCLI(t, "ps", "-d", "self", "--name", "sys"); code != 0 || !strings.Contains(stdout, "System") || !strings.Contains(stdout, "2.0") {
		t.Errorf("ps：%d %q", code, stdout)
	}
	if code, stdout, _ = runCLI(t, "kill", "-d", "self", "--tree", "42"); code != 0 || stdout != "已结束进程：42、43\n" {
		t.Errorf("kill：%d %q", code, stdout)
	}
	if code, _, _ = runCLI(t, "kill", "-d", "self", "abc"); code != exitUsage {
		t.Errorf("kill 非数字 pid：退出码 = %d，期望 2", code)
	}
	for _, args := range [][]string{{"list", "/d"}, {"stat", "/d/a.txt"}, {"info"}, {"ps"}} {
		full := append([]string{args[0], "-d", "self", "--json"}, args[1:]...)
		if code, stdout, _ = runCLI(t, full...); code != 0 || !json.Valid([]byte(stdout)) {
			t.Errorf("%v --json：%d %q", args, code, stdout)
		}
	}
}

func TestCall(t *testing.T) {
	startTestNode(t)
	code, stdout, _ := runCLI(t, "call", "-d", "self", "--tool", "proc.kill", "--args", `{"pid":1}`)
	var resp protocol.CallResponse
	if code != 0 || json.Unmarshal([]byte(stdout), &resp) != nil || !resp.OK {
		t.Errorf("退出码 = %d，标准输出 = %q", code, stdout)
	}
	code, stdout, _ = runCLI(t, "call", "-d", "self", "--tool", "no.such")
	resp = protocol.CallResponse{}
	if code != exitError || json.Unmarshal([]byte(stdout), &resp) != nil || resp.OK || resp.Error == nil || resp.Error.Code != protocol.CodeUnknownTool {
		t.Errorf("未知工具：退出码 = %d，标准输出 = %q", code, stdout)
	}
	if code, _, _ = runCLI(t, "call", "-d", "self", "--tool", "x", "--args", "{bad"); code != exitUsage {
		t.Errorf("非法 JSON：退出码 = %d，期望 2", code)
	}
	if code, _, _ = runCLI(t, "call", "-d", "self"); code != exitUsage {
		t.Errorf("缺 --tool：退出码 = %d，期望 2", code)
	}
}

func TestDeviceSelection(t *testing.T) {
	startTestNode(t)
	code, _, stderr := runCLI(t, "info")
	if code != exitUsage || !strings.Contains(stderr, envDevice) || !strings.Contains(stderr, "deskbridge devices") {
		t.Errorf("缺设备：退出码 = %d，标准错误 = %q", code, stderr)
	}
	t.Setenv(envDevice, "self")
	if code, _, _ = runCLI(t, "info"); code != 0 {
		t.Errorf("环境变量指定设备：退出码 = %d", code)
	}
	t.Setenv(envDevice, "no-such-device")
	if code, _, _ = runCLI(t, "info", "-d", "self"); code != 0 {
		t.Errorf("-d 应优先于环境变量：退出码 = %d", code)
	}
}

func TestJSONErrorOutput(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "info", "-d", "no-such-device", "--json")
	if code != exitError || stderr != "" {
		t.Errorf("退出码 = %d，标准错误 = %q", code, stderr)
	}
	var e struct {
		Error *protocol.Error `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &e); err != nil || e.Error == nil || e.Error.Code != protocol.CodeDeviceNotFound {
		t.Errorf("标准输出 = %q（%v）", stdout, err)
	}
	code, _, stderr = runCLI(t, "info", "-d", "no-such-device")
	if code != exitError || !strings.Contains(stderr, "错误（device_not_found）：") {
		t.Errorf("文本错误：退出码 = %d，标准错误 = %q", code, stderr)
	}
}

func TestNodeNotRunning(t *testing.T) {
	t.Setenv("DESKBRIDGE_HOME", shortTempDir(t))
	t.Setenv(envDevice, "")
	for _, args := range [][]string{{"info", "-d", "self"}, {"exec", "-d", "self", "x"}, {"ping", "self"}} {
		code, _, stderr := runCLI(t, args...)
		if code != exitError || !strings.Contains(stderr, "本机节点未运行") {
			t.Errorf("%v：退出码 = %d，标准错误 = %q", args, code, stderr)
		}
	}
	code, stdout, _ := runCLI(t, "info", "-d", "self", "--json")
	if code != exitError || !json.Valid([]byte(stdout)) {
		t.Errorf("--json：退出码 = %d，标准输出 = %q", code, stdout)
	}
}

func TestPing(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "ping", "desk")
	if code != 0 {
		t.Fatalf("退出码 = %d（%s）", code, stderr)
	}
	if !strings.HasPrefix(stdout, "desk：中继 hkg，延迟 231 毫秒；DeskBridge 响应 ") || !strings.HasSuffix(stdout, " 毫秒\n") {
		t.Errorf("标准输出 = %q", stdout)
	}
	code, stdout, _ = runCLI(t, "ping", "--json", "desk")
	var r struct {
		Peer       mesh.Peer `json:"peer"`
		DeskBridge bool      `json:"deskbridge"`
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &r) != nil || !r.DeskBridge || r.Peer.Relay != "hkg" {
		t.Errorf("--json：退出码 = %d，标准输出 = %q", code, stdout)
	}
	if code, stdout, _ = runCLI(t, "ping", "self"); code != 0 || !strings.Contains(stdout, "本机") {
		t.Errorf("self：退出码 = %d，标准输出 = %q", code, stdout)
	}
	if code, _, stderr = runCLI(t, "ping", "nobody"); code != exitError || !strings.Contains(stderr, "device_not_found") {
		t.Errorf("未知设备：退出码 = %d，标准错误 = %q", code, stderr)
	}
	if code, _, _ = runCLI(t, "ping"); code != exitUsage {
		t.Errorf("缺参数：退出码 = %d，期望 2", code)
	}
}

func TestInitiator(t *testing.T) {
	t.Setenv(envInitiator, "")
	if i := initiator(); i.Type != protocol.InitiatorHuman || i.Via != "cli" {
		t.Errorf("默认发起者 = %+v", i)
	}
	t.Setenv(envInitiator, "ai")
	if i := initiator(); i.Type != protocol.InitiatorAI || i.Via != "cli" {
		t.Errorf("DESKBRIDGE_INITIATOR=ai 时发起者 = %+v", i)
	}
}

func TestDevicesText(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "devices", "--refresh")
	if code != 0 {
		t.Fatalf("退出码 = %d（%s）", code, stderr)
	}
	if !strings.HasPrefix(stdout, "Tailscale（已连接，账号 alice@example.com）\n") {
		t.Errorf("分组标题不对：%q", stdout)
	}
	for _, want := range []string{"win-pc（本机）", "中继 hkg", "231 毫秒", "Tailscale 客户端", "其他 tsnet 程序", "离线", "DeskBridge dev", "连接方式未连接"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("输出缺少 %q：\n%s", want, stdout)
		}
	}
}

func TestDevicesJSON(t *testing.T) {
	startTestNode(t)
	code, stdout, stderr := runCLI(t, "devices", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("退出码 = %d，标准错误 = %q", code, stderr)
	}
	var list client.DeviceList
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("标准输出不是单个 JSON 值：%v", err)
	}
	if len(list.Groups) != 1 || len(list.Groups[0].Devices) != 3 || !list.Groups[0].Devices[0].Self {
		t.Errorf("设备列表 = %+v", list)
	}
	if code, _, _ = runCLI(t, "devices", "extra"); code != exitUsage {
		t.Errorf("多余参数：退出码 = %d，期望 2", code)
	}
}
