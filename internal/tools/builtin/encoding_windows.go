//go:build windows

package builtin

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

// decodeOutput 把子进程的输出解码为 UTF-8（docs/tools.md 编码一节）：
// 去掉 BOM，合法 UTF-8 原样返回，否则按 OEM 代码页解码，再试 ANSI，
// 仍不行就把不合法的字节替换为 U+FFFD。截断留下的半个字符在解码时
// 自然丢弃。
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
	if s, ok := decodeCodePage(b, getOEMCP()); ok {
		return s
	}
	if s, ok := decodeCodePage(b, windows.GetACP()); ok {
		return s
	}
	return strings.ToValidUTF8(string(b), "\uFFFD")
}

// decodeCodePage 用 MultiByteToWideChar 把单字节编码的文本解码为
// UTF-8；字节不是这个代码页的合法文本时返回 false。
func decodeCodePage(b []byte, codePage uint32) (string, bool) {
	if codePage == 0 || codePage == 65001 || len(b) == 0 {
		return "", false // 65001 是 UTF-8，走到这里说明已经不是合法 UTF-8
	}
	n, err := windows.MultiByteToWideChar(codePage, mbErrInvalidChars, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return "", false
	}
	buf := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(codePage, mbErrInvalidChars, &b[0], int32(len(b)), &buf[0], int32(n)); err != nil {
		return "", false
	}
	return string(utf16.Decode(buf)), true
}
