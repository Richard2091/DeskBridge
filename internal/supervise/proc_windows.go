//go:build windows

package supervise

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Prepare 让子进程不弹出控制台窗口（守护进程自己没有控制台时，控制台
// 程序会新开一个窗口）。必须在 Start 之前调用。
func Prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

var (
	jobOnce sync.Once
	job     windows.Handle
	jobErr  error
)

func initJob() {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		jobErr = err
		return
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(h)
		jobErr = err
		return
	}
	job = h
}

// Attach 把已启动的子进程放进作业对象：守护进程退出（包括被强制结束）时，
// 系统会结束子进程，不会留下孤儿节点占着端口。
func Attach(cmd *exec.Cmd) error {
	jobOnce.Do(initJob)
	if jobErr != nil {
		return fmt.Errorf("创建作业对象失败: %w", jobErr)
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("打开子进程失败: %w", err)
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		return fmt.Errorf("加入作业对象失败: %w", err)
	}
	return nil
}
