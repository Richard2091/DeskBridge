//go:build !windows

package builtin

import "os"

// killProcessTree 结束 pid；tree 为 true 时先结束它的后代（由深到浅，
// 去掉节点自己，后代的失败忽略），最后结束根。返回实际结束的 PID。
// err 非 nil 表示根进程没有被结束。
func killProcessTree(pid int, tree, force bool) ([]int, error) {
	var killed []int
	if tree {
		self := os.Getpid()
		var descendants []int
		for _, p := range descendantsOf(pid) {
			if p != self { // 后代里可能有节点自己，绝不能结束
				descendants = append(descendants, p)
			}
		}
		if len(descendants) > 0 {
			// 后代结束失败不算错误，宁可漏杀。
			killed, _ = killProcesses(descendants, force)
		}
	}
	rootKilled, err := killProcesses([]int{pid}, force)
	if err != nil {
		return killed, err
	}
	return append(killed, rootKilled...), nil
}
