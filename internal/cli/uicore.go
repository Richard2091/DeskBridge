package cli

// ui-core 子命令：桌面客户端启动的子进程。在 127.0.0.1 随机端口上提供带令牌的
// HTTP 服务（见 uibridge 包），启动后向标准输出打印一行 JSON
// {"addr":"127.0.0.1:端口","token":"令牌","node":"external|embedded|off"}；
// 标准输入关闭（客户端退出）时自行退出。-node=auto（默认）时，本机已有节点就复用，
// 没有就在本进程内运行节点（embedded），退出时一起关闭。auto 模式下还有一个看护循环：
// 内置节点退出、或复用的节点消失、而本机接口连不上时，重新拉起内置节点。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Richard2091/DeskBridge/internal/logfile"
	"github.com/Richard2091/DeskBridge/internal/nodeconfig"
	"github.com/Richard2091/DeskBridge/internal/paths"
	"github.com/Richard2091/DeskBridge/internal/uibridge"
)

func runUICore(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ui-core", flag.ContinueOnError)
	fs.SetOutput(stderr)
	nodeFlag := fs.String("node", "auto", "本机节点：auto 已有则复用、没有则在本进程内运行；off 只转发")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "未知参数：%s\n", fs.Arg(0))
		return exitUsage
	}
	if *nodeFlag != "auto" && *nodeFlag != "off" {
		fmt.Fprintf(stderr, "-node 只能是 auto 或 off，收到：%s\n", *nodeFlag)
		return exitUsage
	}
	socket, err := paths.SocketPath()
	if err != nil {
		fmt.Fprintf(stderr, "确定本机接口路径失败：%v\n", err)
		return exitError
	}
	token, err := uibridge.NewToken()
	if err != nil {
		fmt.Fprintf(stderr, "生成令牌失败：%v\n", err)
		return exitError
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(stderr, "监听本地端口失败：%v\n", err)
		return exitError
	}
	st := &coreState{}
	st.set("off", "", "")
	srv := &http.Server{Handler: uibridge.HandlerWithCore(socket, token, func() any { return st.snapshot() }), ReadHeaderTimeout: 30 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// 先开始监听标准输入：启动内置节点最长约 19 秒，期间客户端退出要能立刻取消。
	go func() {
		// 客户端退出（含崩溃）时管道关闭，桥接跟着退出，不留孤儿进程。
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	var k *nodeKeeper
	var keeperDone chan struct{}
	if *nodeFlag == "auto" {
		k = newNodeKeeper(socket, st)
		if err := k.step(ctx); err != nil {
			if ctx.Err() != nil {
				k.shutdown(nil, embeddedExitWait)
				return exitOK // 启动期间客户端已退出，安静地结束
			}
			stop()
			k.shutdown(nil, embeddedExitWait)
			fmt.Fprintf(stderr, "本机节点启动失败：%v（日志：%s）\n", err, st.snapshot().Log)
			return exitError
		}
		keeperDone = make(chan struct{})
		go func() {
			defer close(keeperDone)
			k.watch(ctx)
		}()
	}

	line, _ := json.Marshal(map[string]string{"addr": ln.Addr().String(), "token": token, "node": st.snapshot().Mode})
	fmt.Fprintln(stdout, string(line))
	code := exitOK
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "服务中断：%v\n", err)
		code = exitError
	}
	stop()
	if k != nil {
		k.shutdown(keeperDone, embeddedExitWait)
	}
	return code
}

const (
	embeddedReadyWait   = 15 * time.Second // 等内置节点就绪的最长时间
	embeddedStopWait    = 3 * time.Second  // 就绪超时后等节点收尾的最长时间
	embeddedExitWait    = 10 * time.Second // 客户端退出时等节点收尾的最长时间
	keeperInterval      = 2 * time.Second  // 看护循环的检查间隔
	keeperMinBackoff    = 2 * time.Second
	keeperMaxBackoff    = 30 * time.Second
	reachableCheckLimit = time.Second
)

// coreStatus 是 GET /api/core 的返回内容。
type coreStatus struct {
	Mode      string `json:"mode"`       // external | embedded | off | none（暂时没有可用节点）
	NodeError string `json:"node_error"` // 最近一次失败的原因，可为空
	Log       string `json:"log"`        // 节点日志路径
}

// coreState 是并发安全的 coreStatus。
type coreState struct {
	mu sync.Mutex
	s  coreStatus
}

func (c *coreState) set(mode, nodeErr, log string) {
	c.mu.Lock()
	c.s = coreStatus{Mode: mode, NodeError: nodeErr, Log: log}
	c.mu.Unlock()
}

func (c *coreState) snapshot() coreStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.s
}

// waitDone 等 done 关闭，最多等 d。
func waitDone(done <-chan struct{}, d time.Duration) {
	select {
	case <-done:
	case <-time.After(d):
	}
}

// exited 报告节点是否已退出；done 为 nil 表示节点没有启动。
func exited(done <-chan struct{}) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// nodeReachable 报告能否在 timeout 内连上本机接口 socket。
func nodeReachable(socket string, timeout time.Duration) bool {
	c, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// embeddedRun 是一次内置节点的运行。
type embeddedRun struct {
	done    chan struct{} // 节点退出（含收尾）后关闭
	result  chan error    // 节点的退出原因，缓冲 1
	cancel  context.CancelFunc
	logPath string
}

// exitErr 返回节点的退出原因；节点还在运行时返回 nil。
func (r *embeddedRun) exitErr() error {
	select {
	case err := <-r.result:
		if err == nil {
			err = errors.New("节点已退出")
		}
		return err
	default:
		return nil
	}
}

// nodeKeeper 让本机始终有节点可用：复用已有的，没有就在进程内运行，
// 内置节点退出或外部节点消失后重新拉起，连续失败时退避。
type nodeKeeper struct {
	socket string
	st     *coreState

	// 以下三个可在测试中替换。
	reachable func() bool
	launch    func(ctx context.Context) (*embeddedRun, error)
	now       func() time.Time

	mu      sync.Mutex
	cur     *embeddedRun // 正在运行的内置节点
	backoff time.Duration
	nextTry time.Time
}

func newNodeKeeper(socket string, st *coreState) *nodeKeeper {
	k := &nodeKeeper{socket: socket, st: st, backoff: keeperMinBackoff, now: time.Now}
	k.reachable = func() bool { return nodeReachable(socket, reachableCheckLimit) }
	k.launch = launchEmbeddedNode
	return k
}

func (k *nodeKeeper) current() *embeddedRun {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.cur
}

func (k *nodeKeeper) setCurrent(r *embeddedRun) {
	k.mu.Lock()
	k.cur = r
	k.mu.Unlock()
}

// step 做一次检查：必要时启动内置节点。返回的错误是这一步启动内置节点的失败原因
// （本机接口被别人抢先占用而改为复用时不算失败）。
func (k *nodeKeeper) step(ctx context.Context) error {
	if cur := k.current(); cur != nil {
		if !exited(cur.done) {
			return nil
		}
		// 内置节点退出了。它至少成功运行过，退避从头开始。
		reason := cur.exitErr()
		k.setCurrent(nil)
		k.backoff, k.nextTry = keeperMinBackoff, time.Time{}
		k.st.set("none", reason.Error(), cur.logPath)
	}
	if k.reachable() {
		logPath, err := paths.NodeLogFile()
		if err != nil {
			logPath = ""
		}
		k.st.set("external", "", logPath)
		k.backoff, k.nextTry = keeperMinBackoff, time.Time{}
		return nil
	}
	if k.now().Before(k.nextTry) {
		return nil
	}
	run, err := k.launch(ctx)
	if err == nil {
		k.setCurrent(run)
		k.st.set("embedded", "", run.logPath)
		k.backoff, k.nextTry = keeperMinBackoff, time.Time{}
		return nil
	}
	logPath := ""
	if run != nil {
		logPath = run.logPath
	}
	if ctx.Err() == nil && k.reachable() {
		// 同时启动的另一个客户端（或刚自启的节点）抢先占用了本机接口，改为复用。
		return k.step(ctx)
	}
	k.st.set("none", err.Error(), logPath)
	k.nextTry = k.now().Add(k.backoff)
	k.backoff = min(k.backoff*2, keeperMaxBackoff)
	return err
}

// watch 是看护循环，直到 ctx 取消。失败原因已记在状态里，这里不再处理。
func (k *nodeKeeper) watch(ctx context.Context) {
	t := time.NewTicker(keeperInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = k.step(ctx) // 失败原因由 step 写进状态并退避
		}
	}
}

// shutdown 等看护循环（可为 nil）和内置节点收尾，总共最多等 d。调用前 ctx 须已取消。
func (k *nodeKeeper) shutdown(loopDone <-chan struct{}, d time.Duration) {
	deadline := time.Now().Add(d)
	if loopDone != nil {
		waitDone(loopDone, d)
	}
	if cur := k.current(); cur != nil {
		waitDone(cur.done, time.Until(deadline))
	}
}

// launchEmbeddedNode 在 goroutine 里运行节点，等它的本机接口开始监听。
// 失败时也返回 run（可能为 nil），其中的节点已取消并等过收尾。
func launchEmbeddedNode(ctx context.Context) (*embeddedRun, error) {
	dir, err := paths.LogDir()
	if err != nil {
		return nil, fmt.Errorf("确定日志目录失败：%w", err)
	}
	logPath := filepath.Join(dir, "ui-node.log")
	fail := &embeddedRun{logPath: logPath}
	file, err := loadNodeSettings()
	if err != nil {
		return fail, err
	}
	lf, err := logfile.Open(logPath, logfile.DefaultMaxSize, logfile.DefaultBackups)
	if err != nil {
		return fail, fmt.Errorf("打开日志文件失败：%w", err)
	}
	lg := log.New(lf, "", log.LstdFlags)
	logf := func(format string, args ...any) { lg.Printf(format, args...) }

	opts := resolveNodeOptions(file, nodeconfig.Config{}, "")
	ready := make(chan struct{})
	opts.OnListening = func() { close(ready) }
	runCtx, cancel := context.WithCancel(ctx)
	run := &embeddedRun{done: make(chan struct{}), result: make(chan error, 1), cancel: cancel, logPath: logPath}
	go func() {
		defer close(run.done)
		defer lf.Close()
		defer cancel()
		run.result <- runNodeCore(runCtx, opts, logf) // 失败已在日志里记录
	}()

	deadline := time.NewTimer(embeddedReadyWait)
	defer deadline.Stop()
	select {
	case <-ready:
		return run, nil
	case <-run.done:
		err := run.exitErr()
		if err == nil {
			err = errors.New("节点已退出")
		}
		return run, err
	case <-deadline.C:
		cancel()
		waitDone(run.done, embeddedStopWait)
		return run, fmt.Errorf("等待 %d 秒仍未就绪", int(embeddedReadyWait.Seconds()))
	case <-ctx.Done():
		cancel()
		waitDone(run.done, embeddedExitWait)
		return run, errors.New("已取消")
	}
}
