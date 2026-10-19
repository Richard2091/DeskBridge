//go:build windows

package lockfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func acquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件 %s 失败: %w", path, err)
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, fmt.Errorf("%w（%s）", ErrLocked, path)
		}
		return nil, fmt.Errorf("加锁 %s 失败: %w", path, err)
	}
	return func() {
		// 关闭文件即释放锁，进程崩溃时系统也会回收。
		f.Close()
	}, nil
}
