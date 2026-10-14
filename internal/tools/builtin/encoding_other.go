//go:build !windows

package builtin

import (
	"strings"
	"unicode/utf8"
)

// decodeOutput 把子进程的输出解码为 UTF-8。非 Windows 平台的外部程序
// 通常直接输出字节流：去掉 BOM，是合法 UTF-8 就原样返回，否则把不
// 合法的字节替换为 U+FFFD（docs/tools.md 编码一节的第 1、2、3 步）。
func decodeOutput(b []byte) string {
	b = trimUTF8BOM(b)
	// 输出可能在多字节字符中间被截断，先去掉末尾不完整的 UTF-8 字符，
	// 否则整段会被误判为非 UTF-8。
	if t := trimIncompleteUTF8(b); utf8.Valid(t) {
		return string(t)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return strings.ToValidUTF8(string(b), "\uFFFD")
}
