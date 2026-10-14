//go:build !windows && !linux && !darwin

package builtin

import (
	"errors"
	"os/exec"
	"strings"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// 其他平台没有实现任何内置工具；这里的函数只为让包编译通过，
// 对应的工具不会注册，不会被调用。

// procTools 在其他平台不注册任何 proc 工具。
func procTools() []*tools.Tool { return nil }

const defaultShell = ""

func shellAllowed() []string { return nil }

func buildShellCommand(shell, command string) (*exec.Cmd, error) {
	return nil, protocol.Errorf(protocol.CodeUnsupportedPlatform, "当前平台不支持执行命令")
}

func startProcess(cmd *exec.Cmd) (kill, cleanup func(), err error) {
	return nil, nil, errors.New("unsupported_platform")
}

func listProcesses() ([]procEntry, bool) { return nil, false }

func descendantsOf(pid int) []int { return nil }

func killProcesses(pids []int, force bool) ([]int, error) {
	return nil, errors.New("unsupported_platform")
}

func mapKillError(err error) error {
	return protocol.Errorf(protocol.CodeInternal, "%v", err)
}

func decodeOutput(b []byte) string {
	b = trimUTF8BOM(b)
	return strings.ToValidUTF8(string(b), "\uFFFD")
}

func sysPlatform() sysPlatformInfo { return sysPlatformInfo{} }
