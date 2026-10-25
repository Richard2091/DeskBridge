//go:build windows

package browser

// Windows：隐藏控制台窗口；每个浏览器实例放进独立的作业对象，关闭会话时
// 只结束这个实例及其子进程，绝不影响用户正在使用的浏览器窗口。节点退出
// （包括崩溃）时作业句柄关闭，浏览器随之结束，不留孤儿进程。

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processGroup 是一个作业对象，对应一个浏览器实例。
type processGroup struct {
	job windows.Handle
}

// prepareCmd 设置启动属性。
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}

// newProcessGroup 创建一个带「关闭即结束」标志的作业对象。
func newProcessGroup() (*processGroup, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &processGroup{job: h}, nil
}

// attach 把浏览器进程放进作业对象；失败时退化为只结束根进程。
func (g *processGroup) attach(pid int) {
	if g == nil || g.job == 0 || pid <= 0 {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(ph)
	_ = windows.AssignProcessToJobObject(g.job, ph)
}

// terminate 结束这个作业对象里的全部进程。
func (g *processGroup) terminate(cmd *exec.Cmd) {
	if g != nil && g.job != 0 {
		_ = windows.TerminateJobObject(g.job, 0)
	}
	terminateProcess(cmd)
}

// release 关闭作业句柄；句柄关闭时作业对象里的进程被结束。
func (g *processGroup) release() {
	if g == nil || g.job == 0 {
		return
	}
	_ = windows.CloseHandle(g.job)
	g.job = 0
}

// terminateProcess 是退化为只结束根进程时的兜底。
func terminateProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
