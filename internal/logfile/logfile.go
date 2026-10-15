// Package logfile 提供按大小轮转的日志文件写入器。
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// 默认的轮转参数：单个文件最大 5 MiB，保留 3 个旧文件。
const (
	DefaultMaxSize = 5 << 20
	DefaultBackups = 3
)

// Writer 追加写入 path；写入后超过 maxSize 时轮转：
// path.1 是最近的旧文件，path.N 最旧，超出 backups 的删除。
type Writer struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	f       *os.File
	size    int64
}

// Open 打开（必要时创建）日志文件。目录不存在时创建，权限 0700。
// maxSize <= 0 或 backups < 1 时用默认值。
func Open(path string, maxSize int64, backups int) (*Writer, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if backups < 1 {
		backups = DefaultBackups
	}
	w := &Writer{path: path, maxSize: maxSize, backups: backups}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("读取日志文件信息失败: %w", err)
	}
	w.f, w.size = f, st.Size()
	return nil
}

// Write 实现 io.Writer。一次写入不会被拆到两个文件里。
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate 关闭当前文件，把 path.(i) 改名为 path.(i+1)，再重新打开。
// Windows 不能改名已打开的文件，所以先关闭。
func (w *Writer) rotate() error {
	if err := w.f.Close(); err != nil {
		return fmt.Errorf("关闭日志文件失败: %w", err)
	}
	w.f = nil
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.backups))
	for i := w.backups - 1; i >= 1; i-- {
		err := os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("轮转日志失败: %w", err)
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("轮转日志失败: %w", err)
	}
	return w.open()
}

// Close 关闭文件。
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
