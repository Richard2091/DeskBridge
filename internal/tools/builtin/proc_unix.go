//go:build linux || darwin

package builtin

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"sync"
	"syscall"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// userNames 缓存 uid→用户名，避免每个进程都查一遍 /etc/passwd。
var (
	userNamesMu sync.Mutex
	userNames   = map[int]string{}
)

// userNameOf 查 uid 对应的用户名，查不到返回空（结果里省略）。
func userNameOf(uid int) string {
	userNamesMu.Lock()
	defer userNamesMu.Unlock()
	if name, ok := userNames[uid]; ok {
		return name
	}
	name := ""
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		name = u.Username
	}
	userNames[uid] = name
	return name
}

// killProcesses 给目标发信号：force 为 true 发 SIGKILL，否则 SIGTERM。
// 进程已经不在（ESRCH）的处理分三种：
//   - 所有目标都是 ESRCH（一个都没结束）：返回 ESRCH，调用方映射为 not_found；
//   - 部分目标是 ESRCH、其他结束成功（进程树里有成员刚好退出）：算成功，
//     killed 只列实际结束的；
//   - 出现其他错误（如 EPERM）：返回最后一个其他错误，ESRCH 不覆盖它。
func killProcesses(pids []int, force bool) ([]int, error) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	var killed []int
	var lastErr error
	gone := 0
	for _, pid := range pids {
		err := syscall.Kill(pid, sig)
		switch {
		case err == nil:
			killed = append(killed, pid)
		case errors.Is(err, syscall.ESRCH):
			gone++
		default:
			lastErr = err
		}
	}
	if len(killed) == 0 && lastErr == nil && gone > 0 {
		return nil, syscall.ESRCH
	}
	return killed, lastErr
}

// mapKillError 把结束进程的失败映射为协议错误。
func mapKillError(err error) error {
	switch {
	case errors.Is(err, syscall.ESRCH):
		return protocol.Errorf(protocol.CodeNotFound, "进程不存在：%v", err)
	case errors.Is(err, syscall.EPERM):
		return &protocol.Error{
			Code:    protocol.CodePermissionDenied,
			Message: fmt.Sprintf("操作系统拒绝了结束进程：%v", err),
			Details: map[string]any{"missing": "os_permission"},
		}
	default:
		return protocol.Errorf(protocol.CodeInternal, "结束进程失败：%v", err)
	}
}
