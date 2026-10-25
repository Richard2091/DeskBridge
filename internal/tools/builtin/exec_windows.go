//go:build windows

package builtin

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"golang.org/x/sys/windows"
)

// defaultShell 是 proc.exec 未指定 shell 时 Windows 上使用的 shell。
const defaultShell = "powershell"

// shellAllowed 返回 Windows 上允许的 shell。
func shellAllowed() []string { return []string{"powershell", "pwsh", "cmd"} }

// psEncodingPrefix 把 PowerShell 的控制台输出编码设为 UTF-8，加在用户
// 命令前面，见 docs/tools.md 的编码一节。默认不弹窗口时子进程可能没有
// 控制台，设置控制台编码会失败（Windows PowerShell 5.1 会抛“句柄无效”），
// 所以两项都用 try/catch 包住：失败时输出按系统代码页编码，解码侧本来
// 就会按 OEM 代码页回退。
const psEncodingPrefix = `try{[Console]::InputEncoding=[System.Text.UTF8Encoding]::new($false)}catch{};try{[Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false)}catch{};$OutputEncoding=[System.Text.UTF8Encoding]::new($false);`

// consoleHold 是可见模式命令结束后窗口保留的时间，测试里可以改小。
var consoleHold = 5 * time.Second

// helperExecutable 返回 console 子命令所在的程序路径，测试里可以替换。
var helperExecutable = func() (string, error) { return os.Executable() }

// consoleCreationFlags 返回启动子进程用的创建标志。默认不让控制台程序
// 新建窗口（节点自己没有控制台时会弹一个空白窗口）；可见模式反过来，
// 新建一个控制台窗口，标题和回显由 console 子命令负责。
func consoleCreationFlags(visible bool) uint32 {
	if visible {
		return windows.CREATE_NEW_CONSOLE
	}
	return windows.CREATE_NO_WINDOW
}

// buildShellCommand 按 docs/tools.md 的方式构造经 shell 执行的命令。
func buildShellCommand(shell, command string) (*exec.Cmd, error) {
	switch shell {
	case "powershell", "pwsh":
		path, err := exec.LookPath(shell)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeNotFound, "找不到 %s：%v", shell, err)
		}
		// 用 -Command 而不是 -EncodedCommand：后者在输出被重定向时会让
		// PowerShell 把错误流和进度流序列化成 CLIXML 写到 stderr。引号由
		// Go 按 CommandLineToArgvW 规则转义，PowerShell 按同一规则还原，
		// 中文和换行原样保留。前缀后加换行，让错误位置信息不带前缀。
		return &exec.Cmd{
			Path: path,
			Args: []string{shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psEncodingPrefix + "\n" + command},
		}, nil
	case "cmd":
		path, err := exec.LookPath("cmd")
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeNotFound, "找不到 cmd：%v", err)
		}
		// /d 跳过 AutoRun，/s 修改引号处理，/c 执行后退出。命令行自己
		// 拼（SysProcAttr.CmdLine），不让 Go 再转义一次。
		cmd := &exec.Cmd{Path: path, Args: []string{"cmd"}}
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /d /s /c "` + command + `"`}
		return cmd, nil
	default:
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "不支持的 shell：%s", shell)
	}
}

// visibleCommand 把要执行的命令包装成 console 子命令：由它在设备桌面上新建
// 的控制台窗口里运行，窗口里先打印一行模拟人工输入的命令，再把控制台交给
// 真正的命令，命令的输出直接写进窗口。
func visibleCommand(shellCmd *exec.Cmd, display, prompt string) (*exec.Cmd, error) {
	exe, err := helperExecutable()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInternal, "确定程序路径失败：%v", err)
	}
	args := []string{
		"console",
		"-title", consoleTitle(display),
		"-line", echoLine(prompt, display),
		"-exe", shellCmd.Path,
		"-hold", consoleHold.String(),
	}
	if shellCmd.SysProcAttr != nil && shellCmd.SysProcAttr.CmdLine != "" {
		args = append(args, "-cmdline", shellCmd.SysProcAttr.CmdLine)
	} else if len(shellCmd.Args) > 1 {
		args = append(append(args, "--"), shellCmd.Args[1:]...)
	}
	helper := exec.Command(exe, args...)
	helper.Env = shellCmd.Env
	helper.Dir = shellCmd.Dir
	return helper, nil
}

// consoleTitle 生成窗口标题：单行、短，一眼能看出这条命令在做什么。
func consoleTitle(display string) string {
	const max = 80
	s := oneLine(display)
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max]) + "…"
	}
	return "DeskBridge · " + s
}

// echoLine 生成窗口里回显的一行命令；过长的命令截断，避免命令行超限。
func echoLine(prompt, display string) string {
	const max = 400
	s := oneLine(display)
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max]) + "…"
	}
	return prompt + s
}

// oneLine 把命令压成一行（窗口标题和回显都不能带换行）。
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// startProcess 启动进程并放进作业对象，让它创建的子进程默认也在同一
// 作业里。流程：
//  1. 以 CREATE_SUSPENDED 启动，主线程挂起，进程还没执行任何代码，
//     所以它不可能在加入作业之前就创建出不受控的子进程；
//  2. 加入作业对象；加入失败时不中止命令，回退到只能结束主进程
//     （cmd.Process.Kill），保证超时时至少主进程能被结束；
//  3. 枚举该进程的线程（此时只有主线程）并 ResumeThread 恢复执行。
//     选这种方式而不是 NtResumeProcess，是因为 Toolhelp32 和
//     ResumeThread 都是文档化的 API。
//
// 创建标志按 visible 决定：默认 CREATE_NO_WINDOW（不弹窗口），可见模式
// CREATE_NEW_CONSOLE（窗口由 console 子命令持有）。
//
// 作业对象故意不设 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE：命令正常结束
// 后它启动的后台程序（如 start /b、start notepad）要保留。只有超时
// 或取消时，kill 才用 TerminateJobObject 显式结束整棵进程树；cleanup
// 只关闭作业句柄。
func startProcess(cmd *exec.Cmd, visible bool) (kill, cleanup func(), err error) {
	job, jerr := windows.CreateJobObject(nil, nil)
	if jerr != nil {
		return nil, nil, jerr
	}
	// 保留已有的 SysProcAttr（shell=cmd 时设置了 CmdLine），只追加标志。
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | consoleCreationFlags(visible)
	if serr := cmd.Start(); serr != nil {
		windows.CloseHandle(job)
		return nil, nil, serr
	}
	pid := uint32(cmd.Process.Pid)

	inJob := false
	if h, aerr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid); aerr == nil {
		inJob = windows.AssignProcessToJobObject(job, h) == nil
		windows.CloseHandle(h)
	}
	if !inJob {
		// 回退：没有作业对象，只能结束主进程本身。
		windows.CloseHandle(job)
		job = 0
	}
	if rerr := resumeMainThread(pid); rerr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if job != 0 {
			windows.CloseHandle(job)
		}
		return nil, nil, rerr
	}

	var mu sync.Mutex
	closed := false
	closeJob := func() {
		if job != 0 && !closed {
			windows.CloseHandle(job)
		}
		closed = true
	}
	kill = func() {
		mu.Lock()
		defer mu.Unlock()
		if job != 0 && !closed {
			_ = windows.TerminateJobObject(job, 1)
		}
		_ = cmd.Process.Kill() // 作业不可用或主进程不在作业里时的兜底
	}
	cleanup = func() {
		mu.Lock()
		defer mu.Unlock()
		closeJob()
	}
	return kill, cleanup, nil
}

// resumeMainThread 恢复以 CREATE_SUSPENDED 启动的进程的主线程。
func resumeMainThread(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	if err := windows.Thread32First(snap, &te); err != nil {
		return err
	}
	resumed := false
	for {
		if te.OwnerProcessID == pid {
			th, oerr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
			if oerr != nil {
				return oerr
			}
			_, rerr := windows.ResumeThread(th)
			windows.CloseHandle(th)
			if rerr != nil {
				return rerr
			}
			resumed = true
		}
		if err := windows.Thread32Next(snap, &te); err != nil {
			break
		}
	}
	if !resumed {
		return errors.New("找不到新进程的主线程")
	}
	return nil
}
