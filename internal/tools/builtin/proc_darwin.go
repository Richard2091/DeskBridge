//go:build darwin

package builtin

import (
	"errors"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// procTools 返回 macOS 上的 proc 工具。进程列表和进程树的枚举还没有
// 实现，proc.list 和 proc.kill 不注册；proc.exec 可用。
func procTools() []*tools.Tool {
	return []*tools.Tool{newProcExecTool()}
}

// 以下函数只为让包在 macOS 上编译通过；没有工具会调用它们。

func listProcesses() ([]procEntry, bool) { return nil, false }

func descendantsOf(pid int) []int { return nil }

func killProcesses(pids []int, force bool) ([]int, error) {
	return nil, errors.New("unsupported_platform")
}

func mapKillError(err error) error {
	return protocol.Errorf(protocol.CodeInternal, "%v", err)
}
