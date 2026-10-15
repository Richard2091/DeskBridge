package cli

// 工具类子命令（info、exec、read、write 等）的公共部分：解析 -d 和 --json、
// 创建 client、调用工具、统一输出错误。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// 环境变量名。
const (
	envDevice    = "DESKBRIDGE_DEVICE"    // 默认目标设备
	envInitiator = "DESKBRIDGE_INITIATOR" // 设为 ai 时调用发起者类型为 ai
)

// 各子命令用法说明末尾默认的退出码说明。
const exitCodeHelp = "退出码：0 成功；1 工具调用失败或本机节点未运行；2 用法错误。"

// toolCmd 是一个工具类子命令的运行环境。
type toolCmd struct {
	name     string
	fs       *flag.FlagSet
	device   *string
	jsonOut  *bool
	exitHelp string // 用法说明末尾的退出码说明，默认 exitCodeHelp
	stdout   io.Writer
	stderr   io.Writer
}

// newToolCmd 创建子命令的 FlagSet，并加上公共选项 -d 和 --json。
// usage 是用法行（不含“用法：”前缀），extra 是用法行之后的补充说明。
// withJSON 为 false 时不加 --json（call 总是输出 JSON）。
func newToolCmd(name, usage, extra string, withJSON bool, stdout, stderr io.Writer) *toolCmd {
	t := &toolCmd{name: name, fs: flag.NewFlagSet(name, flag.ContinueOnError), stdout: stdout, stderr: stderr, exitHelp: exitCodeHelp}
	t.fs.SetOutput(stderr)
	t.device = t.fs.String("d", "", "目标设备（名称、主机名、内网 IP、连接方式:设备、self），默认取环境变量 "+envDevice)
	t.jsonOut = new(bool)
	if withJSON {
		t.jsonOut = t.fs.Bool("json", false, "输出工具的 result JSON")
	}
	t.fs.Usage = func() {
		fmt.Fprintf(stderr, "用法：deskbridge %s\n", usage)
		if extra != "" {
			fmt.Fprintf(stderr, "\n%s\n", extra)
		}
		fmt.Fprintln(stderr, "\n选项：")
		t.fs.PrintDefaults()
		fmt.Fprintf(stderr, "\n%s\n", t.exitHelp)
	}
	return t
}

// parse 解析参数。返回 ok 为 false 时 code 是应退出的码（-h 为 0，其余为用法错误）。
func (t *toolCmd) parse(args []string) (code int, ok bool) {
	if err := t.fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, false
		}
		return exitUsage, false
	}
	return 0, true
}

// usageError 输出用法错误并返回用法错误退出码。
func (t *toolCmd) usageError(format string, a ...any) int {
	fmt.Fprintf(t.stderr, format+"\n", a...)
	fmt.Fprintf(t.stderr, "运行 deskbridge %s -h 查看用法\n", t.name)
	return exitUsage
}

// target 返回目标设备：-d 优先，其次环境变量；都没有时 ok 为 false 并已输出用法错误。
func (t *toolCmd) target() (string, bool) {
	d := *t.device
	if d == "" {
		d = os.Getenv(envDevice)
	}
	if d == "" {
		t.usageError("没有指定目标设备：用 -d <设备>，或设置环境变量 %s；用 deskbridge devices 查看设备", envDevice)
		return "", false
	}
	return d, true
}

// toolSession 是已确定目标设备、连上本机节点的调用环境。
type toolSession struct {
	*toolCmd
	c      *client.Client
	device string
}

// open 确定目标设备并创建 client。失败时已输出错误，code 为退出码。
func (t *toolCmd) open() (*toolSession, int, bool) {
	dev, ok := t.target()
	if !ok {
		return nil, exitUsage, false
	}
	c, err := localClient()
	if err != nil {
		return nil, t.fail(fmt.Errorf("确定本机接口路径失败：%w", err)), false
	}
	return &toolSession{toolCmd: t, c: c, device: dev}, 0, true
}

// initiator 返回命令行调用的发起者声明。
func initiator() *protocol.Initiator {
	typ := protocol.InitiatorHuman
	if strings.EqualFold(os.Getenv(envInitiator), protocol.InitiatorAI) {
		typ = protocol.InitiatorAI
	}
	return &protocol.Initiator{Type: typ, Via: protocol.ViaCLI}
}

// errorBody 把错误转成 {"error":{...}} 的内容。
func errorBody(err error) *protocol.Error {
	var perr *protocol.Error
	switch {
	case errors.As(err, &perr):
		return perr
	case errors.Is(err, client.ErrNotRunning):
		return &protocol.Error{Code: "node_not_running", Message: "本机节点未运行，先运行 deskbridge node"}
	case errors.Is(err, context.DeadlineExceeded):
		return &protocol.Error{Code: protocol.CodeTimeout, Message: "等待本机节点响应超时（设备可能不可达）"}
	default:
		return &protocol.Error{Code: protocol.CodeInternal, Message: err.Error()}
	}
}

// fail 输出调用失败的错误并返回退出码 1。--json 时输出 {"error":{...}} 到
// 标准输出（保证标准输出只有一个 JSON 值），否则输出到标准错误。
func (t *toolCmd) fail(err error) int {
	body := errorBody(err)
	if *t.jsonOut {
		printJSON(t.stdout, map[string]any{"error": body})
		return exitError
	}
	if reportNotRunning(t.stderr, err) {
		return exitError
	}
	fmt.Fprintf(t.stderr, "错误（%s）：%s\n", body.Code, body.Message)
	return exitError
}

// call 调用目标设备上的工具。TimeoutMS 为 0 时原样传给节点，由节点用
// 工具自己的默认超时；客户端按全局上限加余量等待，避免对端无响应时
// 一直卡住。
func (s *toolSession) call(ctx context.Context, tool string, args any, opts client.CallOptions) (*protocol.CallResponse, error) {
	if opts.Initiator == nil {
		opts.Initiator = initiator()
	}
	return s.c.Call(ctx, s.device, tool, args, opts)
}

// decodeResult 把工具返回的 result 解码到 out。
func decodeResult(raw json.RawMessage, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("解析工具返回值失败：%w", err)
	}
	return nil
}

// callResult 调用工具并把 result 解码到 out（out 为 nil 时不解码）。
// 返回原始 result，--json 时直接输出它。
func (s *toolSession) callResult(tool string, args, out any) (json.RawMessage, error) {
	resp, err := s.call(context.Background(), tool, args, client.CallOptions{})
	if err != nil {
		return nil, err
	}
	if out != nil {
		if err := decodeResult(resp.Result, out); err != nil {
			return nil, err
		}
	}
	return resp.Result, nil
}

// finish 调用工具并输出：--json 时输出 result，否则把 result 解码到 out 后
// 调用 render 输出文本（out 由 render 的闭包使用）。返回退出码。
func (s *toolSession) finish(tool string, args, out any, render func()) int {
	if *s.jsonOut {
		out = nil // --json 只输出原始 result，不需要解码
	}
	raw, err := s.callResult(tool, args, out)
	if err != nil {
		return s.fail(err)
	}
	if *s.jsonOut {
		return s.printRaw(raw)
	}
	render()
	return exitOK
}

// printRaw 把 result 的 JSON 缩进后输出到标准输出。
func (t *toolCmd) printRaw(raw json.RawMessage) int {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		fmt.Fprintf(t.stderr, "输出失败：%v\n", err)
		return exitError
	}
	buf.WriteByte('\n')
	if _, err := t.stdout.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(t.stderr, "输出失败：%v\n", err)
		return exitError
	}
	return exitOK
}

// printJSON 把任意值缩进输出为 JSON（不转义 HTML 字符）。
func printJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	// 写标准输出失败时没有别的渠道可报告，忽略。
	_ = enc.Encode(v)
}

// notifyInterrupt 订阅 Ctrl+C，返回通道和取消订阅的函数；测试中替换。
var notifyInterrupt = func() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return ch, func() { signal.Stop(ch) }
}

// stdinReader 是 --stdin 和 write --from - 读取的标准输入；测试中替换。
var stdinReader io.Reader = os.Stdin
