// Package lockfile 提供跨进程的独占文件锁（非阻塞），用来保证同一数据目录下
// 只运行一个节点。锁随进程退出自动释放。
package lockfile

import "errors"

// ErrLocked 表示锁已被占用（同一进程内的第二次获取也是如此）。
var ErrLocked = errors.New("lockfile: 已被占用")

// Acquire 以非阻塞方式获取 path 上的独占锁，返回释放函数。
// 已被占用时返回的错误满足 errors.Is(err, ErrLocked)。
func Acquire(path string) (release func(), err error) {
	return acquire(path)
}
