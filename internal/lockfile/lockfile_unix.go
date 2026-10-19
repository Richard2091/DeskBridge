//go:build unix

package lockfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// 用 flock 而不是 fcntl 锁：flock 锁在打开的文件描述上，同一进程内第二次
// 打开同一文件再加锁会失败（fcntl 锁按进程算，会重复成功）。
func acquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件 %s 失败: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w（%s）", ErrLocked, path)
		}
		return nil, fmt.Errorf("加锁 %s 失败: %w", path, err)
	}
	return func() {
		// 关闭文件即释放锁，进程崩溃时系统也会回收。
		f.Close()
	}, nil
}
