// Package builtin 实现节点的内置工具（sys.info、fs.*、proc.*）。参数、
// 返回值和错误码的线上格式见 docs/tools.md，实现必须与文档一致；结构
// 字段名即线上的 JSON 键，改动需要考虑兼容性。
package builtin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// Info 是注册内置工具时需要的节点信息。
type Info struct {
	// Name 是设备显示名称（--name），进入 sys.info 的 name 字段。
	Name string
	// Version 是 DeskBridge 的版本，进入 sys.info 的 version 字段。
	Version string
}

// 参数和输出的默认值与上限（docs/tools.md 各工具的表格）。
const (
	defaultListEntries = 1000
	maxListEntries     = 10000
	defaultReadBytes   = 1 << 20 // 1 MiB
	maxReadBytes       = 8 << 20 // 8 MiB
	defaultOutputBytes = 1 << 20 // 1 MiB
	maxOutputBytes     = 8 << 20 // 8 MiB
	defaultProcEntries = 500
	maxProcEntries     = 10000
)

// Register 把当前平台上可用的内置工具注册到 r。某个平台没有实现的
// 工具不注册，能力报告里自然不会出现。
func Register(r *tools.Registry, info Info) error {
	regs := []*tools.Tool{
		newSysInfoTool(info),
		newFsStatTool(),
		newFsListTool(),
		newFsReadTool(),
		newFsWriteTool(),
	}
	regs = append(regs, procTools()...)
	for _, t := range regs {
		if err := r.Register(t); err != nil {
			return fmt.Errorf("注册工具 %s 失败: %w", t.Spec.Name, err)
		}
	}
	return nil
}

// checkCtx 在工具开始工作前检查调用是否已经超时或被取消；已取消时
// 返回 ctx.Err()，由节点协议层映射为 cancelled。
func checkCtx(ctx context.Context) error {
	return ctx.Err()
}

// mapFSError 把文件系统错误映射为协议错误：不存在 → not_found，
// 没有权限 → permission_denied（details.missing 为 os_permission）。
func mapFSError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return protocol.Errorf(protocol.CodeNotFound, "文件或目录不存在：%v", cleanPathErr(err))
	case errors.Is(err, os.ErrPermission):
		return &protocol.Error{
			Code:    protocol.CodePermissionDenied,
			Message: fmt.Sprintf("操作系统拒绝了访问：%v", cleanPathErr(err)),
			Details: map[string]any{"missing": "os_permission"},
		}
	default:
		return protocol.Errorf(protocol.CodeInvalidArgs, "文件操作失败：%v", cleanPathErr(err))
	}
}

// cleanPathErr 去掉错误文本里的包装前缀，只留可读的部分。
func cleanPathErr(err error) string {
	var perr *os.PathError
	if errors.As(err, &perr) {
		return fmt.Sprintf("%s: %v", perr.Path, perr.Err)
	}
	return err.Error()
}

// trimUTF8BOM 去掉开头的 UTF-8 BOM；没有时不改动。
func trimUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// truncateAtRuneBoundary 返回可以整体按文本处理的长度：末尾的多字节
// 字符被截断时回退到字符边界（docs/tools.md fs.read 的规则）。中间的
// 非法字节不回退，交给后续的替换或 base64 判定。
func truncateAtRuneBoundary(b []byte) int {
	n := len(b)
	if n == 0 || utf8.Valid(b) {
		return n
	}
	// 找最后一个多字节序列的起点（跳过末尾的 continuation 字节）。
	i := n - 1
	for i >= 0 && b[i]&0xC0 == 0x80 {
		i--
	}
	if i < 0 {
		return 0
	}
	if _, size := utf8.DecodeRune(b[i:]); size > n-i {
		// 起点声明的序列长度超出剩余字节，说明截断在字符中间。
		return i
	}
	return n
}

// timezoneName 返回 IANA 风格的时区名；取不到时用 UTC 偏移
// （如 +08:00，docs/tools.md sys.info）。
func timezoneName() string {
	name := time.Local.String()
	if name != "" && name != "Local" && !strings.Contains(name, " ") {
		// Linux 不设 TZ 时可能是 /etc/localtime 之类的路径；
		// zoneinfo 路径里可以提取出时区名。
		if i := strings.LastIndex(name, "/zoneinfo/"); i >= 0 {
			return name[i+len("/zoneinfo/"):]
		}
		if !strings.Contains(name, "/") {
			return name
		}
	}
	_, off := time.Now().Zone()
	return formatUTCOffset(off)
}

// formatUTCOffset 把秒数偏移格式化为 +HH:MM 或 -HH:MM。
func formatUTCOffset(sec int) string {
	sign := "+"
	if sec < 0 {
		sign = "-"
		sec = -sec
	}
	return fmt.Sprintf("%s%02d:%02d", sign, sec/3600, sec%3600/60)
}
