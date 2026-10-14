//go:build windows

package builtin

import (
	"encoding/base64"
	"os/exec"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"golang.org/x/sys/windows"
)

// defaultShell 是 proc.exec 未指定 shell 时 Windows 上使用的 shell。
const defaultShell = "powershell"

// shellAllowed 返回 Windows 上允许的 shell。
func shellAllowed() []string { return []string{"powershell", "pwsh", "cmd"} }

// psEncodingPrefix 把 PowerShell 的控制台输出编码设为 UTF-8，加在用户
// 命令前面，见 docs/tools.md 的编码一节。
const psEncodingPrefix = `[Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false);$OutputEncoding=[Console]::OutputEncoding;`

// buildShellCommand 按 docs/tools.md 的方式构造经 shell 执行的命令。
func buildShellCommand(shell, command string) (*exec.Cmd, error) {
	switch shell {
	case "powershell", "pwsh":
		path, err := exec.LookPath(shell)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeNotFound, "找不到 %s：%v", shell, err)
		}
		// 用 -EncodedCommand 传命令，避免引号转义和编码问题。
		encoded := base64.StdEncoding.EncodeToString(utf16LE(psEncodingPrefix + command))
		return &exec.Cmd{
			Path: path,
			Args: []string{shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encoded},
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

// utf16LE 把字符串编码为 UTF-16LE 字节（-EncodedCommand 的格式）。
func utf16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, v := range units {
		b[i*2] = byte(v)
		b[i*2+1] = byte(v >> 8)
	}
	return b
}

// startProcess 启动进程并放进带 KILL_ON_JOB_CLOSE 的作业对象：之后它
// 创建的子进程默认也在同一作业里，超时时一次结束整棵进程树。cleanup
// 在 Wait 之后释放作业对象，kill 在超时或取消时结束整棵树。
func startProcess(cmd *exec.Cmd) (kill, cleanup func(), err error) {
	job, jerr := windows.CreateJobObject(nil, nil)
	if jerr != nil {
		return nil, nil, jerr
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, serr := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); serr != nil {
		windows.CloseHandle(job)
		return nil, nil, serr
	}
	if serr := cmd.Start(); serr != nil {
		windows.CloseHandle(job)
		return nil, nil, serr
	}
	// 主进程显式加入作业；它的子进程会自动继承。进程可能刚好退出，
	// 加入失败也不影响后续流程。
	if h, aerr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); aerr == nil {
		_ = windows.AssignProcessToJobObject(job, h)
		windows.CloseHandle(h)
	}
	var once sync.Once
	kill = func() {
		once.Do(func() {
			windows.TerminateJobObject(job, 1)
			windows.CloseHandle(job)
		})
	}
	cleanup = func() {
		once.Do(func() { windows.CloseHandle(job) })
	}
	return kill, cleanup, nil
}
